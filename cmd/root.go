package cmd

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ffreis/platform-orchestrator/internal/appconfig"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/credential"
	"github.com/ffreis/platform-orchestrator/internal/logger"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/runner"
)

type deps struct {
	cfg    *appconfig.Config
	log    logger.Logger
	cfgctl configctl.Client
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
}

func Execute() error {
	return buildRoot().Execute()
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

	root.AddCommand(
		newInitCmd(d, gf),
		newResumeCmd(d, gf),
		newStatusCmd(d, gf),
		newStepsCmd(d, gf),
	)

	return root
}

func initDeps(ctx context.Context, gf *globalFlags, d *deps) error {
	cfg, err := appconfig.Load()
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

	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	awsOpts := []func(*config.LoadOptions) error{}
	if cfg.AWSRegion != "" {
		awsOpts = append(awsOpts, config.WithRegion(cfg.AWSRegion))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	dynamo := dynamodb.NewFromConfig(awsCfg)
	store := configctl.NewDynamoStore(dynamo, cfg.TableName)

	d.cfg = cfg
	d.log = log
	d.cfgctl = store

	log.Debug("deps initialised", zap.String("table", cfg.TableName))
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

func buildEngine(ctx context.Context, d *deps, gf *globalFlags, dag *pipeline.DAG, runID string) *pipeline.Engine {
	resolver := credential.NewAWSResolver(d.cfg.AWSRegion, runID, d.cfgctl)
	state := pipeline.NewStateStore(d.cfgctl)
	r := runner.NewExecRunner()

	return pipeline.NewEngine(pipeline.EngineOptions{
		DAG:      dag,
		State:    state,
		Resolver: resolver,
		Config:   d.cfgctl,
		Runner:   r,
		Log:      d.log,
		Project:  gf.project,
		Env:      gf.env,
		DryRun:   gf.dryRun,
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

func exitOnError(log logger.Logger, err error) {
	if err != nil {
		if log != nil {
			log.Error("fatal error", zap.Error(err))
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
}
