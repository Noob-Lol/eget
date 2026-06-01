package install

func (p *recordingProgress) Write(data []byte) (int, error) {
	p.bytes += len(data)
	return len(data), nil
}

func (p *recordingProgress) Finish(...string) {
	p.finished = true
}
