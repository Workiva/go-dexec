package dexec

import (
	"context"
	"errors"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"
)

func Test_processContainers(t *testing.T) {
	mockContainer1 := createMockContainer(nil)
	returnMockTaskWithStatus(mockContainer1, containerd.Running, nil)
	// deadlineExceeded
	mockContainer2 := createMockContainer(map[string]string{deadlineLabel: time.Now().Add(-1 * time.Minute).Format(time.RFC3339)})
	returnMockTaskWithStatus(mockContainer2, containerd.Stopped, nil)
	// invalid time, errors +=1 and created +=1
	mockContainer3 := createMockContainer(map[string]string{deadlineLabel: "not-a-real-time"})
	returnMockTaskWithStatus(mockContainer3, containerd.Created, nil)
	// deadline in future, running status
	mockContainer4 := createMockContainer(map[string]string{deadlineLabel: time.Now().Add(1 * time.Minute).Format(time.RFC3339)})
	returnMockTaskWithStatus(mockContainer4, containerd.Running, nil)
	// unknown status
	mockContainer5 := createMockContainer(nil)
	returnMockTaskWithStatus(mockContainer5, containerd.Unknown, nil)
	// error getting status
	mockContainer6 := createMockContainer(nil)
	returnMockTaskWithStatus(mockContainer6, containerd.Unknown, errors.New("unit-test"))
	// error getting task
	mockContainer7 := createMockContainer(nil)
	mockContainer7.On("Task", mock.Anything, mock.Anything).Return(nil, errors.New("unit-test"))
	// not found error getting task
	mockContainer8 := createMockContainer(nil)
	mockContainer8.On("Task", mock.Anything, mock.Anything).Return(nil, errdefs.ErrNotFound)

	mockContainer9 := createMockContainer(nil)
	returnMockTaskWithStatus(mockContainer9, containerd.Pausing, nil)

	mockContainer10 := createMockContainer(nil)
	returnMockTaskWithStatus(mockContainer10, containerd.Paused, nil)

	expected := Stats{
		Running:          2,
		Errors:           3,
		Created:          1,
		Unknown:          1,
		DeadlineExceeded: 1,
		Stopped:          1,
		Pausing:          1,
		Paused:           1,
	}

	containers := []containerd.Container{
		mockContainer1,
		mockContainer2,
		mockContainer3,
		mockContainer4,
		mockContainer5,
		mockContainer6,
		mockContainer7,
		mockContainer8,
		mockContainer9,
		mockContainer10,
	}

	actual := processContainers(context.TODO(), containers)
	assert.Equal(t, expected, actual)
}

func returnMockTaskWithStatus(c *container, status containerd.ProcessStatus, err error) {
	task := new(task)
	c.On("Task", mock.Anything, mock.Anything).Return(task, nil)
	s := containerd.Status{
		Status: status,
	}
	task.On("Status", mock.Anything).Return(s, err)
}

func createMockContainer(labels map[string]string) *container {
	container := new(container)
	container.On("Labels", mock.Anything).Return(labels, nil)
	return container
}
