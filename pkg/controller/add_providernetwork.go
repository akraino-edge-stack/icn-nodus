package controller

import (
	"github.com/akraino-edge-stack/icn-nodus/pkg/controller/providernetwork"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, providernetwork.Add)
}
