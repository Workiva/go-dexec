package dexec

import (
	"errors"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	"regexp"
	"testing"
)

func Test_createTask_run(t *testing.T) {
	mockContainer := new(container)
	mockTask := new(task)
	spec := &oci.Spec{Process: &specs.Process{}}
	mockContainer.
		On("NewTask", mock.Anything, mock.Anything).Return(mockTask, nil).
		On("ID").Return("unit-test").
		On("Spec", mock.Anything).Return(spec, nil)

	mockPs := new(process)
	mockTask.On("Exec", mock.Anything, "unit-test-task", mock.Anything, mock.Anything).Return(mockPs, nil)

	ch := make(<-chan containerd.ExitStatus)
	mockPs.
		On("Wait", mock.Anything).Return(ch, nil).
		On("Start", mock.Anything).Return(nil)

	ct := &createTask{
		container: mockContainer,
	}
	client := new(client)
	client.On("IsServing", mock.Anything).Return(true, nil)
	_ = ct.run(Containerd{ContainerdClient: client}, nil, io.Discard, io.Discard)

	mockContainer.AssertExpectations(t)
	mockTask.AssertExpectations(t)
	mockPs.AssertExpectations(t)
	assert.Equal(t, mockTask, ct.task)
	assert.Equal(t, mockPs, ct.process)
	assert.Equal(t, ch, ct.exitChan)
}

func Test_createTask_generateContainerName(t *testing.T) {
	ct := &createTask{
		opts: CreateTaskOptions{
			CommandDetails: CommandDetails{
				ExecutorId:      2,
				ChainExecutorId: 1,
				ResultId:        3,
			},
		},
	}
	expectedRegex := "chains-1-2-3-[a-zA-Z]{6}"
	containerId := ct.generateContainerName()
	assert.Regexp(t, regexp.MustCompile(expectedRegex), containerId)
}

func Test_createTask_createProcessSpec(t *testing.T) {
	mockContainer := new(container)
	ct := &createTask{
		container: mockContainer,
		cmd:       []string{"java", "-jar", "data-prep-cli.jar"},
		opts: CreateTaskOptions{
			User:       "61000",
			WorkingDir: "/go/src",
		},
	}

	spec := &oci.Spec{Process: &specs.Process{}}
	mockContainer.
		On("Spec", mock.Anything).
		Return(spec, nil)

	ps, _ := ct.createProcessSpec()
	assert.Equal(t, uint32(61000), ps.User.UID)
	assert.Equal(t, ct.opts.WorkingDir, ps.Cwd)
	assert.Equal(t, ps.Args, ct.cmd)
	mockContainer.AssertExpectations(t)
}

func Test_createTask_cleanup_NotFoundErrIgnoredOnTaskDelete(t *testing.T) {
	mockContainer := new(container)
	mockTask := new(task)
	ct := &createTask{container: mockContainer, task: mockTask}

	mockTask.
		On("Delete", mock.Anything, mock.Anything).
		Return(nil, errdefs.ErrNotFound)

	mockContainer.
		On("Delete", mock.Anything, mock.Anything).
		Return(nil)

	err := ct.cleanup(Containerd{})
	assert.Nil(t, err)
	mockTask.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
}

func Test_createTask_cleanup_ErrNotIgnored(t *testing.T) {
	mockContainer := new(container)
	mockTask := new(task)
	ct := &createTask{container: mockContainer, task: mockTask}
	expectedErr := errors.New("unit test")
	mockTask.
		On("Delete", mock.Anything, mock.Anything).
		Return(nil, expectedErr)

	err := ct.cleanup(Containerd{})
	assert.ErrorIs(t, err, expectedErr)
	mockTask.AssertExpectations(t)
	mockContainer.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func Test_createTask_buildCreateContainerArgs(t *testing.T) {
	client := new(client)
	cd := Containerd{
		ContainerdClient: client,
		Namespace:        "k8s.io",
	}

	task := &createTask{
		opts: CreateTaskOptions{
			Mounts: []specs.Mount{
				{
					Destination: "/go/src",
					Source:      "/command/1",
					Type:        "bind",
				},
				{
					Destination: "/opt/libs/1",
					Source:      "/plugin/1/2",
					Type:        "bind",
					Options:     []string{"ro", "ab"},
				},
			},
			Env: []string{
				"A=B",
				"C=D",
			},
			User: "61000",
			CommandDetails: CommandDetails{
				ChainExecutorId: 1,
				ExecutorId:      2,
				ResultId:        3,
			},
			Image: "docker-agent:latest",
		},
		labels: map[string]string{
			"unit-test": "true",
		},
	}

	args := task.buildCreateContainerArgs(cd)
	for i := 0; i < len(args); i++ {
		var assertion func(t assert.TestingT, a any, b any, msgAndArgs ...any) bool = assert.Equal
		var firstArg interface{}
		var secondArg interface{} = args[i]
		switch i {
		case 0:
			firstArg = "--namespace"
		case 1:
			firstArg = cd.Namespace
		case 2:
			firstArg = "create"
		case 3:
			firstArg = "--name"
		case 4:
			firstArg = args[i]
			secondArg = "chains-1-2-3-"
			assertion = assert.Contains
		case 5:
			firstArg = "--user"
		case 6:
			firstArg = "61000"
		case 7, 9:
			firstArg = "-v"
		case 8:
			firstArg = "/command/1:/go/src"
		case 10:
			firstArg = "/plugin/1/2:/opt/libs/1:ro,ab"
		case 11, 13:
			firstArg = "-e"
		case 12:
			firstArg = "A=B"
		case 14:
			firstArg = "C=D"
		case 15:
			firstArg = "--label"
		case 16:
			firstArg = "unit-test=true"
		case 17:
			firstArg = task.opts.Image
		}
		assertion(t, firstArg, secondArg)
	}
}
