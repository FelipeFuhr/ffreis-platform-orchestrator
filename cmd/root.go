package cmd

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/spf13/cobra"

	"github.com/ffreis/platform-orchestrator/internal/appconfig"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/logger"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/runner"
	"github.com/ffreis/platform-orchestrator/internal/ui"
)

type deps struct {
	cfg    *appconfig.Config
	log    *slog.Logger
	cfgctl configctl.Client
	ui     *ui.Presenter
}

type globalFlags struct {
	table          string
	region         string
	logLevel       string
	nonInteractive bool
	confirmAll     bool
	dryRun         bool
	project        string
	env            string
	ui             string
}

var (
	loadAppConfig   = appconfig.Load
	newLogger       = logger.New
	loadAWSConfig   = config.LoadDefaultConfig
	newDynamoClient = dynamodb.NewFromConfig
	newConfigStore  = configctl.NewDynamoStore
	newStateStore   = pipeline.NewStateStore
	newAWSResolver  = credential.NewAWSResolver
	newExecRunner   = runner.NewExecRunner
)

const (
	exitOK    = 0
	exitError = 1
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Execute() int {
	return ExecuteContext(context.Background())
}

// ExecuteContext runs the CLI with the supplied context, so a signal-aware
// caller (main) can cancel the entire command tree on SIGINT/SIGTERM. Cobra
// propagates the context to subcommands via cmd.Context().
func ExecuteContext(ctx context.Context) int {
	return executeCommand(ctx, buildRoot(), os.Stderr)
}

func executeCommand(ctx context.Context, cmd *cobra.Command, stderr io.Writer) int {
	if err := cmd.ExecuteContext(ctx); err != nil {
		if message := err.Error(); message != "" {
			_, _ = io.WriteString(stderr, "error: "+message+"\n")
		}
		return exitCodeForError(err)
	}
	return exitOK
}

func exitCodeForError(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr != nil && exitErr.Code != 0 {
		return exitErr.Code
	}
	return exitError
}

func buildRoot() *cobra.Command {
	gf := &globalFlags{}
	d := &deps{}

	root := &cobra.Command{
		Use:           "platform-orchestrator",
		Short:         "Eliminate manual platform provisioning steps",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initDeps(cmd.Context(), gf, d)
		},
	}

	root.PersistentFlags().StringVar(&gf.table, "table", "", "DynamoDB table name (overrides ORCHESTRATOR_TABLE)")
	root.PersistentFlags().StringVar(&gf.region, "region", "", "AWS region")
	root.PersistentFlags().StringVar(&gf.logLevel, "log-level", "info", "Log level")
	root.PersistentFlags().BoolVar(&gf.nonInteractive, "non-interactive", false, "Disable prompts; fail if any value is missing")
	root.PersistentFlags().BoolVar(&gf.confirmAll, "confirm-all", false, "Auto-confirm all confirmation gates (for CI)")
	root.PersistentFlags().BoolVar(&gf.dryRun, "dry-run", false, "Describe actions without executing them")
	root.PersistentFlags().StringVar(&gf.project, "project", "platform", "Target platform project")
	root.PersistentFlags().StringVar(&gf.env, "env", "", "Target environment (required: dev, staging, prod) — no default to prevent accidental prod targeting")
	root.PersistentFlags().StringVar(&gf.ui, "ui", "auto", "UI mode: auto, plain, rich")

	root.AddCommand(
		newInitCmd(d, gf),
		newResumeCmd(d, gf),
		newStatusCmd(d, gf),
		newStepsCmd(d, gf),
		versionCmd,
	)

	return root
}

func initDeps(ctx context.Context, gf *globalFlags, d *deps) error {
	cfg, err := loadAppConfig()
	if err != nil {
		if gf.table == "" {
			return err
		}
		cfg = &appconfig.Config{TableName: gf.table, LogLevel: gf.logLevel}
	}
	if gf.table != "" {
		cfg.TableName = gf.table
	}
	if gf.region != "" {
		cfg.AWSRegion = gf.region
	}
	if gf.logLevel != "" {
		cfg.LogLevel = gf.logLevel
	}
	cfg.NonInteractive = gf.nonInteractive
	cfg.ConfirmAll = gf.confirmAll
	cfg.DryRun = gf.dryRun

	presenter, err := ui.New(gf.ui)
	if err != nil {
		return fmt.Errorf("init ui: %w", err)
	}

	log, err := newLogger(cfg.LogLevel, presenter.Interactive())
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	awsOpts := []func(*config.LoadOptions) error{}
	if cfg.AWSRegion != "" {
		awsOpts = append(awsOpts, config.WithRegion(cfg.AWSRegion))
	}
	awsCfg, err := loadAWSConfig(ctx, awsOpts...)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	dynamo := newDynamoClient(awsCfg)
	store := newConfigStore(dynamo, cfg.TableName)

	d.cfg = cfg
	d.log = log
	d.cfgctl = store
	d.ui = presenter

	log.Debug("deps initialised", "table", cfg.TableName)
	return nil
}

func requireProjectEnv(gf *globalFlags) error {
	if gf.project == "" {
		return fmt.Errorf("--project is required")
	}
	if gf.env == "" {
		return fmt.Errorf("--env is required")
	}
	return nil
}

func buildEngine(ctx context.Context, d *deps, gf *globalFlags, dag *pipeline.DAG, runID string, progressOut io.Writer) *pipeline.Engine {
	resolver := newAWSResolver(d.cfg.AWSRegion, runID, d.cfgctl)
	state := newStateStore(d.cfgctl)
	r := newExecRunner()

	return pipeline.NewEngine(pipeline.EngineOptions{
		DAG:         dag,
		State:       state,
		Resolver:    resolver,
		Config:      d.cfgctl,
		Runner:      r,
		Log:         d.log,
		ProgressOut: progressOut,
		UI:          d.ui,
		Project:     gf.project,
		Env:         gf.env,
		DryRun:      gf.dryRun,
	})
}

func newRunID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

	const length = 12
	b := make([]byte, length)
	max := byte(256 - (256 % len(chars)))

	for i := 0; i < length; i++ {
		for {
			var buf [1]byte
			if _, err := rand.Read(buf[:]); err != nil {
				// Extremely unlikely; fall back to a timestamp-based ID.
				return fmt.Sprintf("run-%s-%d", time.Now().UTC().Format("20060102T150405.000000000Z"), os.Getpid())
			}
			if buf[0] >= max {
				continue
			}
			b[i] = chars[int(buf[0])%len(chars)]
			break
		}
	}
	return string(b)
}
