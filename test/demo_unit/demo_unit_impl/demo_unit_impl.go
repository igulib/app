package demo_unit_impl

import (
	"github.com/igulib/app"
)

type DemoUnitImpl struct {
	runner       *app.UnitLifecycleRunner
	availability app.UnitAvailability
}

func NewDemoUnit(name string) *DemoUnitImpl {
	u := &DemoUnitImpl{
		runner: app.NewUnitLifecycleRunner(name),
	}
	u.runner.SetOwner(u)
	return u
}

// Implement IUnit interface
func (u *DemoUnitImpl) UnitRunner() *app.UnitLifecycleRunner {
	return u.runner
}

func (u *DemoUnitImpl) UnitStart() app.UnitOperationResult {
	return app.UnitOperationResult{OK: true, CollateralError: nil}
}

func (u *DemoUnitImpl) UnitPause() app.UnitOperationResult {
	return app.UnitOperationResult{OK: true, CollateralError: nil}
}

func (u *DemoUnitImpl) UnitQuit() app.UnitOperationResult {

	return app.UnitOperationResult{OK: true, CollateralError: nil}
}

func (u *DemoUnitImpl) UnitAvailability() app.UnitAvailability {
	return u.availability
}
