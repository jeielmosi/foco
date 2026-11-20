package retrieve

import (
	"testing"

	"foco/internal/enums"
)

func TestIssueSetKeepsFirstSeenOrderAndProviderMessage(t *testing.T) {
	set := &issueSet{counts: map[enums.RetrievalIssueCode]int{}}
	set.add(enums.IssueCodeChanged)
	set.addProvider("HTTP 503: unavailable")
	set.add(enums.IssueCodeChanged)
	set.add(enums.IssueCodeResourceLimit)

	got := set.list()
	want := []Issue{{enums.IssueCodeChanged, 2}, {enums.IssueCodeProvider, 1}, {enums.IssueCodeResourceLimit, 1}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("issue[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if set.providerFailure != "HTTP 503: unavailable" {
		t.Fatalf("provider failure = %q", set.providerFailure)
	}
}

func TestIssueSetZeroValue(t *testing.T) {
	var set issueSet
	set.add(enums.IssueCodeChanged)

	got := set.list()
	want := []Issue{{enums.IssueCodeChanged, 1}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("issue[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestIssueSetCountsProviderPerOccurrence(t *testing.T) {
	set := &issueSet{}
	set.add(enums.IssueCodeProvider)
	set.addProvider("boom")

	got := set.list()
	want := []Issue{{enums.IssueCodeProvider, 2}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("issue[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if set.providerFailure != "boom" {
		t.Fatalf("provider failure = %q", set.providerFailure)
	}
}

func TestIssueSetIgnoresInvalidCodes(t *testing.T) {
	set := &issueSet{}
	set.add(enums.UnknownRetrievalIssueCode)
	set.add(enums.RetrievalIssueCode("not-a-valid-issue"))
	if got := set.list(); len(got) != 0 {
		t.Fatalf("issues = %+v, want no issues", got)
	}
}

func TestRetrievalIssueMappings(t *testing.T) {
	fileIssues := map[enums.FileIssueKind]enums.RetrievalIssueCode{
		enums.FileIssueUnreadable:    enums.IssueCodeUnreadable,
		enums.FileIssueChanged:       enums.IssueCodeChanged,
		enums.FileIssueResourceLimit: enums.IssueCodeResourceLimit,
		enums.FileIssueInterrupted:   enums.IssueCodeInterrupted,
		enums.FileIssueDetector:      enums.IssueCodeDetector,
	}
	for fileIssue, want := range fileIssues {
		if got := retrievalIssueForFileIssue(fileIssue); got != want {
			t.Errorf("retrievalIssueForFileIssue(%q) = %q, want %q", fileIssue, got, want)
		}
	}
	if got := retrievalIssueForFileIssue(enums.UnknownFileIssueKind); got.IsValid() {
		t.Errorf("unknown file issue mapped to valid code %q", got)
	}

	providerFailures := map[enums.ProviderFailureKind]enums.RetrievalIssueCode{
		enums.FailureAuthentication: enums.IssueCodeAuthentication,
		enums.FailureRequestLimit:   enums.IssueCodeRequestLimit,
		enums.FailureProvider:       enums.IssueCodeProvider,
		enums.FailureCancelled:      enums.IssueCodeCancelled,
		enums.FailureSourceInvalid:  enums.UnknownRetrievalIssueCode,
	}
	for failure, want := range providerFailures {
		if got := retrievalIssueForProviderFailure(failure); got != want {
			t.Errorf("retrievalIssueForProviderFailure(%q) = %q, want %q", failure, got, want)
		}
	}
}

func TestFatalIssue(t *testing.T) {
	tests := []struct {
		kind enums.RetrievalIssueCode
		want bool
	}{
		{enums.IssueCodeAuthentication, true},
		{enums.IssueCodeRequestLimit, true},
		{enums.IssueCodeCancelled, true},
		{enums.IssueCodeInterrupted, true},
		{enums.IssueCodeProvider, false},
		{enums.IssueCodeResourceLimit, false},
		{enums.UnknownRetrievalIssueCode, false},
	}
	for _, test := range tests {
		if got := fatalIssue(test.kind); got != test.want {
			t.Errorf("fatalIssue(%q) = %v, want %v", test.kind.String(), got, test.want)
		}
	}
}
