package util

import "io"

// StdioConn is a wrapper that combines an io.ReadCloser and an io.WriteCloser
// into a single io.ReadWriteCloser, primarily used for IPC via stdio.
type StdioConn struct {
	io.ReadCloser
	io.WriteCloser
}

// Read delegates to the underlying ReadCloser.
func (c *StdioConn) Read(p []byte) (n int, err error) {
	return c.ReadCloser.Read(p)
}

// Write delegates to the underlying WriteCloser.
func (c *StdioConn) Write(p []byte) (n int, err error) {
	return c.WriteCloser.Write(p)
}

// Close closes both the ReadCloser and WriteCloser.
func (c *StdioConn) Close() error {
	err1 := c.ReadCloser.Close()
	err2 := c.WriteCloser.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
