package api

import "testing"

func TestContainsNoSuchBucket(t *testing.T){
	cases := []struct{ in string; want bool }{
		{"NoSuchBucket: bucket does not exist", true},
		{"The specified bucket does not exist", true},
		{"permission denied", false},
	}
	for _, c := range cases {
		if got := containsNoSuchBucket(c.in); got != c.want { t.Fatalf("%q => %v (want %v)", c.in, got, c.want) }
	}
}
