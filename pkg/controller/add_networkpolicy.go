package controller

import (
	"github.com/akraino-edge-stack/icn-nodus/pkg/controller/networkpolicy"
	// "github.com/akraino-edge-stack/icn-nodus/pkg/controller/networkpolicypod"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, networkpolicy.Add)
	// AddToManagerFuncs = append(AddToManagerFuncs, networkpolicypod.Add)
}
