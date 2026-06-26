package proxy

import "testing"

func TestProxyAuthenticationFailuresKeepCompatibleStageAndMessages(t *testing.T) {
	cases := []struct {
		name string
		got  Failure
		want Failure
	}{
		{
			name: "missing proxy authentication",
			got:  MissingProxyAuthenticationFailure(),
			want: Failure{Stage: FailureStageAuthentication, Error: "proxy authentication required"},
		},
		{
			name: "invalid proxy authentication",
			got:  InvalidProxyAuthenticationFailure(),
			want: Failure{Stage: FailureStageAuthentication, Error: "invalid proxy authentication"},
		},
		{
			name: "invalid proxy credentials",
			got:  InvalidProxyCredentialsFailure(),
			want: Failure{Stage: FailureStageAuthentication, Error: "invalid proxy credentials"},
		},
		{
			name: "access profile not found",
			got:  AccessProfileNotFoundFailure(),
			want: Failure{Stage: FailureStageProfileSelection, Error: "access profile not found"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("failure = %#v, want %#v", tt.got, tt.want)
			}
		})
	}
}

func TestClassifyProxyPathFailureKeepsCompatibleSelectionStages(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want Failure
	}{
		{
			name: "profile selection",
			err:  "access profile not found",
			want: Failure{Stage: FailureStageProfileSelection, Error: "access profile not found"},
		},
		{
			name: "path selection",
			err:  "access profile has no usable proxy path",
			want: Failure{Stage: FailureStagePathSelection, Error: "access profile has no usable proxy path"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyProxyPathFailure(tt.err)
			if got != tt.want {
				t.Fatalf("failure = %#v, want %#v", got, tt.want)
			}
		})
	}
}
