package dexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/namespaces"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	randomSuffixLength = 6
	timeoutBuffer      = 5 * time.Minute
	nerdctlBinary      = "nerdctl"

	chains                 = "chains"
	ownerLabel             = "wk/owner"
	deadlineLabel          = "chains/deadline"
	commandExecutorIdLabel = "chains/commandExecutorId"
	chainExecutorIdLabel   = "chains/chainExecutorId"
	commandResultIdLabel   = "chains/commandResultId"
)

type CreateTaskOptions struct {
	Image          string
	Mounts         []specs.Mount
	User           string
	Env            []string
	CommandTimeout time.Duration
	WorkingDir     string
	CommandDetails CommandDetails
}

func ByCreatingTask(opts CreateTaskOptions, logger *logrus.Entry) (Execution[Containerd], error) {
	return &createTask{opts: opts, logger: logger}, nil
}

type createTask struct {
	opts        CreateTaskOptions
	ctx         context.Context
	doneFunc    func(ctx context.Context) error
	image       containerd.Image
	container   containerd.Container
	task        containerd.Task
	cmd         []string
	process     containerd.Process
	exitChan    <-chan containerd.ExitStatus
	tmpDir      string
	logger      *logrus.Entry
	labels      map[string]string
	expiration  time.Duration
	transaction *newrelic.Transaction
}

func (t *createTask) setTransaction(txn *newrelic.Transaction) {
	t.transaction = txn
}

func (t *createTask) create(c Containerd, cmd []string) error {
	t.cmd = cmd
	// add buffer to the command timeout
	expiration := t.opts.CommandTimeout + timeoutBuffer
	t.expiration = expiration
	// the default containerd settings makes things eligible for garbage collection after 24 hours
	// since we are spinning up hundreds of thousands of tasks per day, let's set a shorter expiration
	// so we can try and be good netizens
	ctx := namespaces.WithNamespace(context.Background(), c.Namespace)
	ctx, deleteLease, err := c.WithLease(ctx, leases.WithExpiration(expiration), leases.WithRandomID())
	if err != nil {
		return fmt.Errorf("error creating containerd context: %w", err)
	}

	t.ctx = ctx
	t.doneFunc = deleteLease

	t.buildLabels()

	container, err := t.createContainer(c)

	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}
	t.container = container

	return nil
}

// createContainer creates a running container on the containerd host but does not start it. Containerd is different
// from Docker in that the client is a fat client. When making calls on the socket, some actions happen on the running
// container, while others happen on the host. By default, if you create a container using the socket, there is NO
// networking configured. It is _extremely_ complicated to get it working, _especially_ in a configuration where you
// are using a mounted socket and trying to create containers and tasks on the host. As a workaround for some of the
// complexity, we are using the nerdctl binary to create the container itself. When nerdctl creates the container, it
// adds hooks to the container's spec that are executed by the host to set up the networking and any other required
// infrastructure. Once the container is successfully created by nerdctl, we then use the socket to create tasks, run
// them, and wait for completion
func (t *createTask) createContainer(c Containerd) (containerd.Container, error) {
	defer t.transaction.StartSegment("createContainer").End()
	defer func(start time.Time) {
		dur := time.Now().Sub(start).Milliseconds()
		t.logger.WithField("duration", dur).Debugf("dexec: entire create container operation took: %d ms", dur)
	}(time.Now())
	nerdctlArgs := t.buildCreateContainerArgs(c)
	containerId, err := t.executeCreateContainer(nerdctlArgs...)
	if err != nil {
		return nil, fmt.Errorf("nerdctl: error creating container: %w", err)
	}

	ctx := namespaces.WithNamespace(context.Background(), c.Namespace)

	return t.loadContainer(ctx, c, containerId)
}

func (t *createTask) executeCreateContainer(args ...string) (containerId string, err error) {
	defer t.transaction.StartSegment("executeCreateContainer").End()
	defer func(start time.Time) {
		if err == nil {
			dur := time.Now().Sub(start).Milliseconds()
			t.logger.WithField("duration", dur).Debugf("nerdctl created container '%s' in %d ms", containerId, dur)
		}
	}(time.Now())
	cmd := exec.Command(nerdctlBinary, args...)
	stdout := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stdErr
	if err = cmd.Run(); err != nil {
		return "", err
	}
	containerId = strings.TrimSpace(stdout.String())
	return containerId, nil
}

func (t *createTask) loadContainer(ctx context.Context, c Containerd, containerId string) (container containerd.Container, err error) {
	defer t.transaction.StartSegment("loadContainer").End()
	defer func(start time.Time) {
		if err == nil {
			dur := time.Now().Sub(start).Milliseconds()
			t.logger.Debugf("LoadContainer operation took %d ms", dur)
		}
	}(time.Now())
	container, err = c.LoadContainer(ctx, containerId)
	return container, err
}
func (t *createTask) buildCreateContainerArgs(c Containerd) []string {
	defer t.transaction.StartSegment("buildCreateContainerArgs").End()
	args := []string{"--namespace", c.Namespace, "create", "--name", t.generateContainerName(), "--user", t.opts.User}
	for _, m := range t.opts.Mounts {
		args = append(args, "-v", fmt.Sprintf("%s:%s", m.Source, m.Destination))
	}
	for _, e := range t.opts.Env {
		args = append(args, "-e", e)
	}
	for key, value := range t.labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}
	args = append(args, t.opts.Image)
	return args
}

func (t *createTask) generateContainerName() string {
	// AA: in order to prevent errors such as being unable to re-run a command due to a failure
	// or timing issue when cleaning up a prior attempt, append a random suffix to the end to make
	// sure we can always create the container
	suffix := RandomString(randomSuffixLength)
	details := t.opts.CommandDetails
	// IDs can't have two hyphens in a row, so we use abs to generate a compliant id for the health check containers
	return fmt.Sprintf("chains-%d-%d-%d-%s", abs(details.ChainExecutorId), abs(details.ExecutorId), abs(details.ResultId), suffix)
}

func (t *createTask) buildLabels() {
	labels := make(map[string]string)

	labels[ownerLabel] = chains
	labels[commandExecutorIdLabel] = strconv.FormatInt(t.opts.CommandDetails.ExecutorId, 10)
	labels[chainExecutorIdLabel] = strconv.FormatInt(t.opts.CommandDetails.ChainExecutorId, 10)
	labels[commandResultIdLabel] = strconv.FormatInt(t.opts.CommandDetails.ResultId, 10)

	if deadline, ok := t.ctx.Deadline(); ok {
		labels[deadlineLabel] = deadline.Format(time.RFC3339)
	}

	t.labels = labels
}

func abs(v int64) int64 {
	if v >= 0 {
		return v
	}
	f := math.Abs(float64(v))
	return int64(f)
}

func (t *createTask) run(c Containerd, stdin io.Reader, stdout, stderr io.Writer) error {
	if err := t.ensureConnection(c); err != nil {
		return err
	}
	opts := []cio.Opt{cio.WithStreams(stdin, stdout, stderr)}
	task, err := t.createTask(opts...)
	if err != nil {
		return fmt.Errorf("error creating task: %w", err)
	}

	t.task = task

	spec, err := t.createProcessSpec()
	if err != nil {
		return fmt.Errorf("error creating process spec: %w", err)
	}
	taskId := fmt.Sprintf("%s-task", t.container.ID())
	ps, err := task.Exec(t.ctx, taskId, spec, cio.NewCreator(opts...))
	if err != nil {
		return fmt.Errorf("error creating process: %w", err)
	}
	t.process = ps

	// wait must always be called before start()
	t.exitChan, err = ps.Wait(t.ctx)
	if err != nil {
		return fmt.Errorf("error waiting for process: %w", err)
	}

	if err = ps.Start(t.ctx); err != nil {
		return fmt.Errorf("error starting process: %w", err)
	}
	return nil
}

func (t *createTask) ensureConnection(c Containerd) error {
	defer t.transaction.StartSegment("ensureConnection").End()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if isServing, err := c.IsServing(ctx); !isServing || err != nil {
		t.logger.Warnf("grpc is not currently serving connection or returned an error while checking. isServing: %t, err: %v", isServing, err)
		if err = c.Reconnect(); err != nil {
			return fmt.Errorf("error ensuring grpc connection: %w", err)
		}
	}
	return nil
}
func (t *createTask) createTask(opts ...cio.Opt) (containerd.Task, error) {
	defer t.transaction.StartSegment("createTask").End()
	return t.container.NewTask(t.ctx, cio.NewCreator(opts...))
}

func (t *createTask) createProcessSpec() (*specs.Process, error) {
	defer t.transaction.StartSegment("createProcessSpec").End()
	spec, err := t.container.Spec(t.ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting spec from container: %w", err)
	}

	spec.Process.Args = t.cmd
	spec.Process.Cwd = t.opts.WorkingDir
	if uid, err := strconv.ParseInt(t.opts.User, 10, 64); err == nil {
		spec.Process.User.UID = uint32(uid)
	}
	return spec.Process, nil
}

func (t *createTask) wait(c Containerd) (int, error) {
	defer t.cleanup(c)

	select {
	case exitStatus := <-t.exitChan:
		return int(exitStatus.ExitCode()), exitStatus.Error()
	case <-time.After(t.expiration):
		t.logger.Warn("time expired before receiving exit status from container/task")
		return -1, context.Canceled
	}
}

func (t *createTask) setEnv(env []string) error {
	if len(t.opts.Env) > 0 {
		return errors.New("dexec: Config.Env already set")
	}
	t.opts.Env = env
	return nil
}

func (t *createTask) setDir(dir string) error {
	if t.opts.WorkingDir != "" {
		return errors.New("dexec: Config.WorkingDir already set")
	}
	t.opts.WorkingDir = dir
	return nil
}

func (t *createTask) getID() string {
	return t.container.ID()
}

// kill kills the running task and cleans up any resources that were created to run it. For all intents and purposes
// kill is identical to cleanup
func (t *createTask) kill(c Containerd) error {
	return t.cleanup(c)
}

// cleanup kills any tasks that are still running, deletes them, and deletes the container that ran the task. if the
// api returns a NotFound error, the error is ignored and we will return nil. otherwise, any errors encountered during
// the cleanup operations will be returned
func (t *createTask) cleanup(Containerd) error {
	defer func() {
		if f := t.doneFunc; f != nil && t.ctx != nil {
			f(t.ctx)
		}
	}()
	_, err := t.task.Delete(t.ctx, containerd.WithProcessKill)
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("error deleting task: %w", err)
	}
	if err = t.container.Delete(t.ctx, containerd.WithSnapshotCleanup); err == nil || errdefs.IsNotFound(err) {
		return nil
	}
	return fmt.Errorf("error deleting container: %w", err)
}
