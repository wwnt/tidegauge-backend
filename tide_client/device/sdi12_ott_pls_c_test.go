package device

import (
	"math"
	"testing"
)

// registeredPLSClimits returns the driver instance that was actually registered, so the
// assertions below stay honest if the registered ranges ever change.
func registeredPLSClimits(t *testing.T) *plsC {
	t.Helper()

	d, ok := getRegisteredDevice("PLS-C").(*plsC)
	if !ok {
		t.Fatal("PLS-C is not registered as a *plsC")
	}
	return d
}

func TestPLSRegisteredLimitsAcceptEveryHardwareVariant(t *testing.T) {
	t.Parallel()

	d := registeredPLSClimits(t)

	// The widest pressure cell OTT sells is 0...100 m water column, so its full
	// scale must be inside the registered level window.
	if filterPLSValue("water_level", new(100.0), d.minLevel, d.maxLevel) == nil {
		t.Fatalf("registered level window [%v, %v] rejects the 100 m variant full scale",
			d.minLevel, d.maxLevel)
	}

	// Full scale of the widest conductivity sub-range (0.1...100 mS/cm).
	if filterPLSValue("water_conductivity", new(100.0), d.minCond, d.maxCond) == nil {
		t.Fatalf("registered conductivity window [%v, %v] rejects 100 mS/cm",
			d.minCond, d.maxCond)
	}
	// Low end of the probe's first measuring range (5 uS/cm == 0.005 mS/cm).
	if filterPLSValue("water_conductivity", new(0.005), d.minCond, d.maxCond) == nil {
		t.Fatalf("registered conductivity window [%v, %v] rejects 5 uS/cm",
			d.minCond, d.maxCond)
	}

	// Fresh water reports exactly zero salinity and TDS, and 60 is the full scale
	// of the ppt salinity range.
	for _, v := range []float64{0.0, 60.0} {
		if filterPLSValue("water_salinity", new(v), d.minSal, d.maxSal) == nil {
			t.Fatalf("registered salinity window [%v, %v] rejects %v", d.minSal, d.maxSal, v)
		}
		if filterPLSValue("water_total_dissolved_solids", new(v), d.minTDS, d.maxTDS) == nil {
			t.Fatalf("registered TDS window [%v, %v] rejects %v", d.minTDS, d.maxTDS, v)
		}
	}

	// The window must be ordered, otherwise every reading would be discarded.
	for _, w := range []struct {
		name     string
		min, max float64
	}{
		{"water_level", d.minLevel, d.maxLevel},
		{"water_temperature", d.minTemp, d.maxTemp},
		{"water_conductivity", d.minCond, d.maxCond},
		{"water_salinity", d.minSal, d.maxSal},
		{"water_total_dissolved_solids", d.minTDS, d.maxTDS},
	} {
		if w.min > w.max {
			t.Fatalf("registered %s window is inverted: min %v > max %v", w.name, w.min, w.max)
		}
	}
}

// TestPLSFilterIsInclusiveAtBounds pins the closed-interval behaviour: a reading exactly
// at a limit is kept, just outside it is dropped. The previous strict "<" comparisons
// silently discarded boundary readings such as a full-scale level or a zero salinity.
func TestPLSFilterIsInclusiveAtBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		v        float64
		min, max float64
		want     bool
	}{
		{name: "belowMin", v: -0.1, min: 0, max: 10, want: false},
		{name: "atMin", v: 0, min: 0, max: 10, want: true},
		{name: "inside", v: 5, min: 0, max: 10, want: true},
		{name: "atMax", v: 10, min: 0, max: 10, want: true},
		{name: "aboveMax", v: 10.1, min: 0, max: 10, want: false},
		{name: "equalBounds", v: 3, min: 3, max: 3, want: true},
		{name: "nan", v: math.NaN(), min: 0, max: 10, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterPLSValue("test_item", new(tt.v), tt.min, tt.max)
			if (got != nil) != tt.want {
				t.Fatalf("filterPLSValue(%v, [%v, %v]) kept=%v, want kept=%v",
					tt.v, tt.min, tt.max, got != nil, tt.want)
			}
		})
	}
}

func TestPLSFilterRejectsImplausibleValues(t *testing.T) {
	t.Parallel()

	d := registeredPLSClimits(t)

	if got := filterPLSValue("water_level", new(d.maxLevel+0.001), d.minLevel, d.maxLevel); got != nil {
		t.Fatalf("filterPLSValue kept an over-range level of %v", *got)
	}
	if got := filterPLSValue("water_level", nil, d.minLevel, d.maxLevel); got != nil {
		t.Fatalf("filterPLSValue turned a missing value into %v", *got)
	}

	// A pass-through value must keep pointing at the same reading.
	val := new(1.5)
	if got := filterPLSValue("water_level", val, d.minLevel, d.maxLevel); got != val {
		t.Fatal("filterPLSValue did not return the original pointer for a plausible value")
	}
}
