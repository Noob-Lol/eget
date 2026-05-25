package app

type SelfReplaceResult struct {
	Deferred bool
}

type ExecutableReplacer interface {
	Replace(currentPath, replacementPath string) (SelfReplaceResult, error)
}
