package admin

import "testing"

func TestCoerceBackupBoolCellValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		in      any
		want    any
		wantErr bool
	}{
		{name: "nil preserved", in: nil, want: nil},
		{name: "bool true", in: true, want: true},
		{name: "bool false", in: false, want: false},
		{name: "sqlite float64 0", in: float64(0), want: false},
		{name: "sqlite float64 1", in: float64(1), want: true},
		{name: "sqlite int64 1", in: int64(1), want: true},
		{name: "invalid float64 2", in: float64(2), wantErr: true},
		{name: "invalid string", in: "true", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := coerceBackupBoolCellValue("use_system_proxy", tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %v, got %v", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
