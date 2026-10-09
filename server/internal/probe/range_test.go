package probe

import "testing"

func TestDynamicRange(t *testing.T) {
	for _, c := range []struct {
		info Info
		want string
	}{
		{Info{}, ""},
		{Info{ColorTransfer: "bt709"}, ""},
		{Info{ColorTransfer: "smpte2084"}, "hdr10"},
		{Info{ColorTransfer: "arib-std-b67"}, "hlg"},
		{Info{ColorTransfer: "smpte2084", DVProfile: 8}, "dv"},
		{Info{DVProfile: 5}, "dv"}, // profile 5 has no HDR10 layer
	} {
		if got := c.info.DynamicRange(); got != c.want {
			t.Errorf("%+v: %q, want %q", c.info, got, c.want)
		}
	}
}
