package dexec

import (
	"context"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/leases"
)

// ContainerdClient is the interface spec of all the calls we use
// from *containerd.Client . Having the interface makes it easier to mock the
// client in tests
type ContainerdClient interface {
	NewContainer(context.Context, string, ...containerd.NewContainerOpts) (containerd.Container, error)
	GetImage(context.Context, string) (containerd.Image, error)
	WithLease(context.Context, ...leases.Opt) (context.Context, func(ctx context.Context) error, error)
	IsServing(context.Context) (bool, error)
	LoadContainer(context.Context, string) (containerd.Container, error)
	Containers(context.Context, ...string) ([]containerd.Container, error)
	Reconnect() error
}

type Containerd struct {
	ContainerdClient
	Namespace string
}

type ContainerdCmd struct {
	*GenericCmd[Containerd]
}

func (c Containerd) Command(method Execution[Containerd], name string, arg ...string) *ContainerdCmd {
	return &ContainerdCmd{
		GenericCmd: &GenericCmd[Containerd]{
			Path:   name,
			Args:   arg,
			Method: method,
			client: c,
		},
	}
}
