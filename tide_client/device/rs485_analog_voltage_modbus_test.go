package device

import (
	"math"
	"testing"
)

func TestDecodeAnalogVoltageModbusValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload []byte
		want    float64
	}{
		{name: "oneVolt", payload: []byte{0x79, 0x18}, want: 1},
		{name: "zero", payload: []byte{0x00, 0x00}, want: 0},
		{name: "integer", payload: []byte{0x04, 0xD2}, want: 1234},
		{name: "fractional", payload: []byte{0x52, 0xF2}, want: 12.34},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeAnalogVoltageModbusValue(tt.payload)
			if err != nil {
				t.Fatalf("decodeAnalogVoltageModbusValue returned error: %v", err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("decodeAnalogVoltageModbusValue(%v) = %v, want %v", tt.payload, got, tt.want)
			}
		})
	}
}

func TestDecodeAnalogVoltageModbusValueRejectsShortPayload(t *testing.T) {
	t.Parallel()

	if _, err := decodeAnalogVoltageModbusValue([]byte{0x01}); err == nil {
		t.Fatal("decodeAnalogVoltageModbusValue should reject payloads that are not 2 bytes")
	}
}
