package cmd

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/spf13/cobra"

	"github.com/ffreis/platform-orchestrator/internal/appconfig"
	"github.com/ffreis/platform-orchestrator/internal/configctl"
	"github.com/ffreis/platform-orchestrator/internal/logger"
	"github.com/ffreis/platform-orchestrator/internal/pipeline"
	"github.com/ffreis/platform-orchestrator/internal/prompt"
)

type fakeDynamoClient struct{}

type fakeStatusStateStore struct {
	lastRunID string
	err       error
}

func (f fakeStatusStateStore) LastRunID(context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.lastRunID, nil
}

func (fakeDynamoClient) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func (fakeDynamoClient) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}

func (fakeDynamoClient) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return &dynamodb.DeleteItemOutput{}, nil
}

func (fakeDynamoClient) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil
}

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe(): %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	fn()
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	return buf.String()
}

func TestExecute_Help(t *testing.T) {
	root := buildRoot()
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

func TestInitDeps_SuccessAndFallback(t *testing.T) {
	origLoad := loadAppConfig
	origLogger := newLogger
	origAWS := loadAWSConfig
	origDyn := newDynamoClient
	origStore := newConfigStore
	t.Cleanup(func() {
		loadAppConfig = origLoad
		newLogger = origLogger
		loadAWSConfig = origAWS
		newDynamoClient = origDyn
		newConfigStore = origStore
	})

	loadAppConfig = func() (*appconfig.Config, error) {
		return &appconfig.Config{TableName: "tbl", AWSRegion: "us-east-1", LogLevel: "info"}, nil
	}
	newLogger = func(string, bool) (*slog.Logger, error) { return logger.Nop(), nil }
	loadAWSConfig = func(context.Context, ...func(*awscfg.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	newDynamoClient = func(aws.Config, ...func(*dynamodb.Options)) *dynamodb.Client { return &dynamodb.Client{} }
	newConfigStore = func(configctl.DynamoClient, string) *configctl.DynamoStore {
		return configctl.NewDynamoStore(fakeDynamoClient{}, "tbl")
	}

	d := &deps{}
	gf := &globalFlags{project: "platform", env: "dev", dryRun: true}
	if err := initDeps(context.Background(), gf, d); err != nil {
		t.Fatalf("initDeps() error: %v", err)
	}
	if d.cfg == nil || d.log == nil || d.cfgctl == nil {
		t.Fatalf("unexpected deps: %+v", d)
	}

	loadAppConfig = func() (*appconfig.Config, error) { return nil, appconfig.ErrMissingTable }
	gf.table = "fallback"
	if err := initDeps(context.Background(), gf, d); err != nil {
		t.Fatalf("initDeps() fallback error: %v", err)
	}
	if d.cfg.TableName != "fallback" {
		t.Fatalf("expected fallback table, got %q", d.cfg.TableName)
	}
}

func TestInitDeps_Errors(t *testing.T) {
	origLoad := loadAppConfig
	origLogger := newLogger
	origAWS := loadAWSConfig
	t.Cleanup(func() {
		loadAppConfig = origLoad
		newLogger = origLogger
		loadAWSConfig = origAWS
	})

	loadAppConfig = func() (*appconfig.Config, error) { return nil, appconfig.ErrMissingTable }
	if err := initDeps(context.Background(), &globalFlags{}, &deps{}); !errors.Is(err, appconfig.ErrMissingTable) {
		t.Fatalf("unexpected error: %v", err)
	}

	loadAppConfig = func() (*appconfig.Config, error) {
		return &appconfig.Config{TableName: "tbl", LogLevel: "info"}, nil
	}
	newLogger = func(string, bool) (*slog.Logger, error) { return nil, errors.New("log") }
	if err := initDeps(context.Background(), &globalFlags{}, &deps{}); err == nil || !strings.Contains(err.Error(), "init logger: log") {
		t.Fatalf("unexpected logger error: %v", err)
	}

	newLogger = func(string, bool) (*slog.Logger, error) { return logger.Nop(), nil }
	loadAWSConfig = func(context.Context, ...func(*awscfg.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("aws")
	}
	if err := initDeps(context.Background(), &globalFlags{}, &deps{}); err == nil || !strings.Contains(err.Error(), "load AWS config: aws") {
		t.Fatalf("unexpected aws error: %v", err)
	}
}

func TestNewStepsCmd_PrintsPipeline(t *testing.T) {
	origPipeline := buildPlatformSetupPipeline
	t.Cleanup(func() { buildPlatformSetupPipeline = origPipeline })

	buildPlatformSetupPipeline = func(configctl.Client) (*pipeline.DAG, error) {
		dag := pipeline.NewDAG()
		_ = dag.Add(&stubStep{id: "step-a", specs: nil})
		return dag, nil
	}

	out := captureOutput(t, func() {
		cmd := newStepsCmd(&deps{cfgctl: newMemConfig()}, &globalFlags{})
		cmd.SetContext(context.Background())
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("RunE() error: %v", err)
		}
	})
	if !strings.Contains(out, "ORDER") || !strings.Contains(out, "step-a") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNewStatusCmd_PrintsStoredState(t *testing.T) {
	cfg := newMemConfig()
	state := pipeline.NewStateStore(cfg)
	meta := &pipeline.RunMeta{RunID: "run-1", Project: "platform", Env: "dev", StartedAt: time.Now(), Status: pipeline.StatusRunning}
	if err := state.SaveRunMeta(context.Background(), meta); err != nil {
		t.Fatalf("SaveRunMeta(): %v", err)
	}
	if err := state.SaveStepState(context.Background(), "run-1", &pipeline.StepState{
		StepID: "step-a", Status: pipeline.StatusSucceeded, Attempts: 1, StartedAt: time.Now(), FinishedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveStepState(): %v", err)
	}

	out := captureOutput(t, func() {
		cmd := newStatusCmd(&deps{cfgctl: cfg, log: logger.Nop()}, &globalFlags{})
		cmd.SetContext(context.Background())
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("RunE() error: %v", err)
		}
	})
	if !strings.Contains(out, "STEP") || !strings.Contains(out, "step-a") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestResolveStatusRunID(t *testing.T) {
	t.Run("uses explicit run id", func(t *testing.T) {
		got, err := resolveStatusRunID(context.Background(), fakeStatusStateStore{lastRunID: "latest"}, "run-1")
		if err != nil {
			t.Fatalf("resolveStatusRunID() error: %v", err)
		}
		if got != "run-1" {
			t.Fatalf("resolveStatusRunID() = %q", got)
		}
	})

	t.Run("falls back to last run id", func(t *testing.T) {
		got, err := resolveStatusRunID(context.Background(), fakeStatusStateStore{lastRunID: "latest"}, "")
		if err != nil {
			t.Fatalf("resolveStatusRunID() error: %v", err)
		}
		if got != "latest" {
			t.Fatalf("resolveStatusRunID() = %q", got)
		}
	})

	t.Run("returns friendly error", func(t *testing.T) {
		_, err := resolveStatusRunID(context.Background(), fakeStatusStateStore{err: errors.New("missing")}, "")
		if err == nil || !strings.Contains(err.Error(), "no run ID provided") {
			t.Fatalf("resolveStatusRunID() error = %v", err)
		}
	})
}

func TestStatusHelpers(t *testing.T) {
	if got := formatStatusTime(time.Time{}); got != "-" {
		t.Fatalf("formatStatusTime(zero) = %q", got)
	}

	value := time.Date(2026, 3, 31, 12, 30, 45, 0, time.UTC)
	if got := formatStatusTime(value); got != "2026-03-31T12:30:45" {
		t.Fatalf("formatStatusTime() = %q", got)
	}

	var out bytes.Buffer
	states := map[string]*pipeline.StepState{
		"step-a": {
			StepID:     "step-a",
			Status:     pipeline.StatusSucceeded,
			Attempts:   2,
			StartedAt:  value,
			FinishedAt: value,
		},
	}
	if err := writeStatusTable(&commandOutput{out: &out}, states, nil); err != nil {
		t.Fatalf("writeStatusTable() error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "STEP") || !strings.Contains(output, "step-a") {
		t.Fatalf("unexpected table output: %q", output)
	}
}

func TestRunInit_SuccessAndRunnerFailure(t *testing.T) {
	origPipeline := buildPlatformSetupPipeline
	t.Cleanup(func() { buildPlatformSetupPipeline = origPipeline })

	buildPlatformSetupPipeline = func(configctl.Client) (*pipeline.DAG, error) {
		dag := pipeline.NewDAG()
		return dag, nil
	}

	d := &deps{
		cfg:    &appconfig.Config{NonInteractive: true, AWSRegion: "us-east-1"},
		log:    logger.Nop(),
		cfgctl: newMemConfig(),
	}
	gf := &globalFlags{project: "platform", env: "dev"}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	if err := runInit(cmd, d, gf, &runFlags{}); err != nil {
		t.Fatalf("runInit() error: %v", err)
	}

	tmp := t.TempDir()
	failScript := filepath.Join(tmp, "fail.sh")
	if err := os.WriteFile(failScript, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	err := runInit(cmd, d, gf, &runFlags{run: true, runnerBinary: failScript})
	if err == nil || !strings.Contains(err.Error(), "platform-runner") {
		t.Fatalf("unexpected runner error: %v", err)
	}
}

func TestCollectInputs_InteractiveAndError(t *testing.T) {
	origInteractive := newInteractivePrompter
	origBatch := newBatchPrompter
	origCollector := newCollector
	t.Cleanup(func() {
		newInteractivePrompter = origInteractive
		newBatchPrompter = origBatch
		newCollector = origCollector
	})

	dag := pipeline.NewDAG()
	_ = dag.Add(&stubStep{id: "step-a"})
	d := &deps{cfg: &appconfig.Config{NonInteractive: false}, cfgctl: newMemConfig(), log: logger.Nop()}
	gf := &globalFlags{project: "platform", env: "dev"}

	var gotCollectorProject, gotCollectorEnv string
	newInteractivePrompter = prompt.NewInteractivePrompter
	newCollector = func(client configctl.Client, pr prompt.Prompter, project, env string) *prompt.Collector {
		gotCollectorProject, gotCollectorEnv = project, env
		return prompt.NewCollector(client, pr, project, env)
	}
	if err := collectInputs(context.Background(), nil, d, gf, dag); err != nil {
		t.Fatalf("collectInputs() error: %v", err)
	}
	if gotCollectorProject != "platform" || gotCollectorEnv != "dev" {
		t.Fatalf("unexpected collector args: %s %s", gotCollectorProject, gotCollectorEnv)
	}

}

func TestNewResumeCmd_Run(t *testing.T) {
	origPipeline := buildPlatformSetupPipeline
	t.Cleanup(func() { buildPlatformSetupPipeline = origPipeline })
	buildPlatformSetupPipeline = func(configctl.Client) (*pipeline.DAG, error) { return pipeline.NewDAG(), nil }

	cfg := newMemConfig()
	state := pipeline.NewStateStore(cfg)
	if err := state.SaveRunMeta(context.Background(), &pipeline.RunMeta{RunID: "run-1", Project: "platform", Env: "dev"}); err != nil {
		t.Fatalf("SaveRunMeta(): %v", err)
	}

	d := &deps{cfg: &appconfig.Config{AWSRegion: "us-east-1", NonInteractive: true}, cfgctl: cfg, log: logger.Nop()}
	gf := &globalFlags{project: "platform", env: "dev"}
	cmd := newResumeCmd(d, gf)
	cmd.SetContext(context.Background())
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE() error: %v", err)
	}
}

func TestExecuteAndExitOnError(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"platform-orchestrator", "--help"}
	t.Cleanup(func() { os.Args = oldArgs })
	if code := Execute(); code != exitOK {
		t.Fatalf("Execute() code = %d, want %d", code, exitOK)
	}
}

func TestExecuteCommand_ReturnsExitCodeAndErrorText(t *testing.T) {
	t.Parallel()

	command := &cobra.Command{
		RunE: func(*cobra.Command, []string) error {
			return &ExitError{Code: 7, Err: errors.New("boom")}
		},
	}

	var stderr bytes.Buffer
	code := executeCommand(context.Background(), command, &stderr)
	if code != 7 {
		t.Fatalf("executeCommand() code = %d, want 7", code)
	}
	if got := stderr.String(); got != "error: boom\n" {
		t.Fatalf("executeCommand() stderr = %q", got)
	}
}
