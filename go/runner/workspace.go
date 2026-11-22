package runner

// LockWorkspace shares the runner's cross-platform live-process check and crash
// recovery with a single model-free Agent tool invocation.
func LockWorkspace(root string) (release func(), err error) {
	lock, err := acquire(root)
	if err != nil {
		return nil, err
	}
	return lock.release, nil
}
