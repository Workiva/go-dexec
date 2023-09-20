package dexec

import (
	"context"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/mock"
)

type client struct {
	mock.Mock
	ContainerdClient
}

func (c *client) IsServing(ctx context.Context) (bool, error) {
	args := c.Called(ctx)
	err := args.Error(1)
	if s, ok := args.Get(0).(bool); ok {
		return s, err
	}
	return false, err
}

type container struct {
	mock.Mock
	containerd.Container
}

func (c *container) Labels(ctx context.Context) (map[string]string, error) {
	args := c.Called(ctx)
	err := args.Error(1)
	if labels, ok := args.Get(0).(map[string]string); ok {
		return labels, err
	}
	return nil, err
}

func (c *container) Task(ctx context.Context, attach cio.Attach) (containerd.Task, error) {
	args := c.Called(ctx, attach)
	err := args.Error(1)
	if task, ok := args.Get(0).(containerd.Task); ok {
		return task, err
	}
	return nil, err
}

func (c *container) NewTask(ctx context.Context, creator cio.Creator, opts ...containerd.NewTaskOpts) (containerd.Task, error) {
	args := c.Called(ctx, creator)
	err := args.Error(1)
	if taskIfc, ok := args.Get(0).(containerd.Task); ok {
		return taskIfc, err
	}
	return nil, err
}

func (c *container) Spec(ctx context.Context) (*oci.Spec, error) {
	args := c.Called(ctx)
	err := args.Error(1)
	if spec, ok := args.Get(0).(*oci.Spec); ok {
		return spec, err
	}
	return nil, err
}

func (c *container) Delete(ctx context.Context, opts ...containerd.DeleteOpts) error {
	inputArgs := make([]interface{}, 0, 1+len(opts))
	inputArgs = append(inputArgs, ctx)
	for _, opt := range opts {
		inputArgs = append(inputArgs, opt)
	}
	args := c.Called(inputArgs)
	return args.Error(0)
}

func (c *container) ID() string {
	return c.Called().String(0)
}

type task struct {
	mock.Mock
	containerd.Task
}

func (t *task) Exec(ctx context.Context, id string, spec *specs.Process, creator cio.Creator) (containerd.Process, error) {
	args := t.Called(ctx, id, spec, creator)
	err := args.Error(1)
	if ps, ok := args.Get(0).(containerd.Process); ok {
		return ps, err
	}
	return nil, err
}

func (t *task) Delete(ctx context.Context, opts ...containerd.ProcessDeleteOpts) (*containerd.ExitStatus, error) {
	inputArgs := make([]interface{}, 0, 1+len(opts))
	inputArgs = append(inputArgs, ctx)
	for _, o := range opts {
		inputArgs = append(inputArgs, o)
	}
	args := t.Called(inputArgs...)
	err := args.Error(1)
	if es, ok := args.Get(0).(*containerd.ExitStatus); ok {
		return es, err
	}
	return nil, err
}

func (t *task) Status(ctx context.Context) (containerd.Status, error) {
	args := t.Called(ctx)
	err := args.Error(1)
	if status, ok := args.Get(0).(containerd.Status); ok {
		return status, err
	}
	return containerd.Status{}, err
}

type process struct {
	mock.Mock
	containerd.Process
}

func (p *process) Wait(ctx context.Context) (<-chan containerd.ExitStatus, error) {
	args := p.Called(ctx)
	err := args.Error(1)
	if ch, ok := args.Get(0).(<-chan containerd.ExitStatus); ok {
		return ch, err
	}
	return nil, err
}

func (p *process) Start(ctx context.Context) error {
	args := p.Called(ctx)
	return args.Error(0)
}
