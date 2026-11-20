package enums

import "testing"

type stringEnum interface {
	~string
	IsValid() bool
	String() string
}

func assertEnum[T stringEnum](
	t *testing.T,
	valid []T,
	unknown T,
	newValue func(string) (T, bool),
) {
	t.Helper()
	for _, want := range valid {
		if !want.IsValid() || want.String() != string(want) {
			t.Fatalf("valid value %q: IsValid=%v String=%q", want, want.IsValid(), want.String())
		}
		got, ok := newValue(string(want))
		if !ok || got != want {
			t.Fatalf("New(%q) = %q, %v; want %q, true", want, got, ok, want)
		}
	}
	if unknown.IsValid() || unknown.String() != "" {
		t.Fatalf("unknown value: IsValid=%v String=%q", unknown.IsValid(), unknown.String())
	}
	invalid := T("not-a-known-value")
	if invalid.IsValid() || invalid.String() != "" {
		t.Fatalf("invalid conversion: IsValid=%v String=%q", invalid.IsValid(), invalid.String())
	}
	for _, input := range []string{"", "not-a-known-value"} {
		got, ok := newValue(input)
		if ok || got != unknown {
			t.Fatalf("New(%q) = %q, %v; want %q, false", input, got, ok, unknown)
		}
	}
}

func TestSharedStringEnums(t *testing.T) {
	assertEnum(t, []Schema{SchemaSystemOne, SchemaGemini, SchemaOpenAI}, UnknownSchema, NewSchema)
	assertEnum(t, []ProviderFailureKind{FailureAuthentication, FailureRequestLimit, FailureProvider, FailureCancelled, FailureSourceInvalid}, UnknownProviderFailureKind, NewProviderFailureKind)
	assertEnum(t, []RetrievalStatus{RetrievalStatusComplete, RetrievalStatusIncomplete, RetrievalStatusInterrupted}, UnknownRetrievalStatus, NewRetrievalStatus)
	assertEnum(t, []FileCategory{FileCategoryCode, FileCategoryConfig, FileCategoryDocs, FileCategoryData, FileCategoryAudio, FileCategoryVideo, FileCategoryArchives, FileCategoryLogs, FileCategoryTemp, FileCategoryUnknown}, UnknownFileCategory, NewFileCategory)
	assertEnum(t, []FileReason{FileReasonOutsideRoot, FileReasonProtected, FileReasonHidden, FileReasonSensitiveName, FileReasonSymlink, FileReasonSpecialFile, FileReasonDependency, FileReasonIgnored, FileReasonExcludePattern, FileReasonNotDirectory, FileReasonMissing, FileReasonNotFile, FileReasonBinary, FileReasonInvalidUTF8, FileReasonSensitiveContent, FileReasonFileTypeCode, FileReasonFileTypeConfig, FileReasonFileTypeDocs, FileReasonFileTypeData, FileReasonFileTypeAudio, FileReasonFileTypeVideo, FileReasonFileTypeArchives, FileReasonFileTypeLogs, FileReasonFileTypeTemp, FileReasonFileTypeUnknown}, UnknownFileReason, NewFileReason)
	assertEnum(t, []FileIssueKind{FileIssueUnreadable, FileIssueChanged, FileIssueResourceLimit, FileIssueInterrupted, FileIssueDetector}, UnknownFileIssueKind, NewFileIssueKind)
	assertEnum(t, []LookupStatus{LookupStatusFile, LookupStatusExcluded}, UnknownLookupStatus, NewLookupStatus)
	assertEnum(t, []SnapshotStatus{SnapshotStatusSnapshot, SnapshotStatusExcluded, SnapshotStatusIssue}, UnknownSnapshotStatus, NewSnapshotStatus)
	assertEnum(t, []NavigationKind{NavigationKindFile, NavigationKindDirectory}, UnknownNavigationKind, NewNavigationKind)
	assertEnum(t, []AssessmentRole{AssessmentRoleImplementation, AssessmentRoleCaller, AssessmentRoleTest, AssessmentRoleFixture, AssessmentRoleHelper}, UnknownAssessmentRole, NewAssessmentRole)
	assertEnum(t, []RetrievalIssueCode{IssueCodeAuthentication, IssueCodeRequestLimit, IssueCodeProvider, IssueCodeCancelled, IssueCodeInterrupted, IssueCodeUnreadable, IssueCodeChanged, IssueCodeResourceLimit, IssueCodeDetector, IssueCodeRequestSize, IssueCodeLowConfidenceBudget, IssueCodeSourceInspectionLimit}, UnknownRetrievalIssueCode, NewRetrievalIssueCode)
}

func TestNewSchemaRequiresExactValue(t *testing.T) {
	if got, ok := NewSchema(" system-one "); ok || got != UnknownSchema {
		t.Fatalf("NewSchema with surrounding spaces = %q, %v; want %q, false", got, ok, UnknownSchema)
	}
}
