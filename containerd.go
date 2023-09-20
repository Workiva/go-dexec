package dexec

import (
	"context"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/leases"
)

type ContainerdClient interface {
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
