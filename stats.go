package dexec

type Stats struct {
	Running          int
	Created          int
	Stopped          int
	Paused           int
	Pausing          int
	Unknown          int
	DeadlineExceeded int
	Errors           int
}

func GetStats(client interface{}) (Stats, error) {
	switch c := client.(type) {
	case Containerd:
		return getContainerdStats(c)
	default:
		return Stats{}, nil
	}
}
