// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type FactoryTest struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FactoryTest{})

func (s *FactoryTest) TestFatal(c *gc.C) {
	c.Fatalf("XXX")
}
