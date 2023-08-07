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
func (u *DemoUnitImpl) Runner() *app.UnitLifecycleRunner {
	return u.runner
}

func (u *DemoUnitImpl) StartUnit() app.UnitOperationResult {
	return app.UnitOperationResult{OK: true, CollateralError: nil}
}

func (u *DemoUnitImpl) PauseUnit() app.UnitOperationResult {
	return app.UnitOperationResult{OK: true, CollateralError: nil}
}

func (u *DemoUnitImpl) QuitUnit() app.UnitOperationResult {

	return app.UnitOperationResult{OK: true, CollateralError: nil}
}

func (u *DemoUnitImpl) UnitAvailability() app.UnitAvailability {
	return u.availability
}
