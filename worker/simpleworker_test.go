// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

type simpleWorkerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&simpleWorkerSuite{})

func (s *simpleWorkerSuite) TestWait(c *gc.C) {
	doWork := func(_ <-chan struct{}) error {
		return testError
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), gc.Equals, testError)
}

func (s *simpleWorkerSuite) TestWaitNil(c *gc.C) {
	doWork := func(_ <-chan struct{}) error {
		return nil
	}

	w := NewSimpleWorker(doWork)
	c.Assert(w.Wait(), gc.Equals, nil)
}

func (s *simpleWorkerSuite) TestKill(c *gc.C) {
	doWork := func(stopCh <-chan struct{}) error {
		<-stopCh
		return testError
	}

	w := NewSimpleWorker(doWork)
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, testError)

	// test we can kill again without a panic
	w.Kill()
}
