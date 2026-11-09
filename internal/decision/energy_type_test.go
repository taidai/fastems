package decision

import "testing"

// 通用化验证：同一套 EnergyUnit+Capability+分配，靠方向约束表达不同资源。
// 光伏只能放电、充电桩只能充电、储能双向，而不是写死三套逻辑。
func TestEnergyTypeDirectionConstraint(t *testing.T) {
	caps := CapabilityValues{
		"pv":      {FieldRatedPower: 100, FieldAvailablePower: 100, FieldEnergyType: EnergyPVDischarge},
		"ev":      {FieldRatedPower: 100, FieldAvailablePower: 100, FieldEnergyType: EnergyEVCharge},
		"storage": {FieldRatedPower: 100, FieldAvailablePower: 100, FieldEnergyType: EnergyStorageBidir},
	}
	members := []EnergyMember{{EnergyUnitID: "pv"}, {EnergyUnitID: "ev"}, {EnergyUnitID: "storage"}}

	t.Run("PV excluded from charge request", func(t *testing.T) {
		// 充电请求（负值）：光伏不能充电，应被剔除；充电桩/储能可充。
		allocations, err := AllocateEnergy(-50, members, caps)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range allocations {
			if a.EnergyUnitID == "pv" && !a.Excluded {
				t.Fatal("光伏不应接受充电请求（只能放电）")
			}
		}
	})

	t.Run("EV excluded from discharge request", func(t *testing.T) {
		// 放电请求（正值）：充电桩不能放电，应被剔除；光伏/储能可放。
		allocations, err := AllocateEnergy(50, members, caps)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range allocations {
			if a.EnergyUnitID == "ev" && !a.Excluded {
				t.Fatal("充电桩不应接受放电请求（只能充电）")
			}
		}
	})

	t.Run("storage bidirectional", func(t *testing.T) {
		for _, request := range []float64{30, -30} {
			allocations, err := AllocateEnergy(request, []EnergyMember{{EnergyUnitID: "storage"}}, caps)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range allocations {
				if a.EnergyUnitID == "storage" && a.Excluded {
					t.Fatalf("储能应双向可调，请求 %v 被剔除", request)
				}
			}
		}
	})
}
