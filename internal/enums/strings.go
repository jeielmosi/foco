package enums

func oneOf[T ~string](value T, valid ...T) bool {
	for _, candidate := range valid {
		if value == candidate {
			return true
		}
	}
	return false
}

func parseEnum[T ~string](input string, unknown T, valid ...T) (T, bool) {
	value := T(input)
	if oneOf(value, valid...) {
		return value, true
	}
	return unknown, false
}

type Schema string

const (
	UnknownSchema   Schema = ""
	SchemaSystemOne Schema = "system-one"
	SchemaGemini    Schema = "gemini-decisions"
	SchemaOpenAI    Schema = "openai-decisions"
)

func (value Schema) IsValid() bool {
	return oneOf(value, SchemaSystemOne, SchemaGemini, SchemaOpenAI)
}

func (value Schema) String() string {
	if !value.IsValid() {
		return string(UnknownSchema)
	}
	return string(value)
}

func NewSchema(input string) (Schema, bool) {
	return parseEnum(input, UnknownSchema, SchemaSystemOne, SchemaGemini, SchemaOpenAI)
}

type ProviderFailureKind string

const (
	UnknownProviderFailureKind ProviderFailureKind = ""
	FailureAuthentication      ProviderFailureKind = "authentication"
	FailureRequestLimit        ProviderFailureKind = "request-limit"
	FailureProvider            ProviderFailureKind = "provider"
	FailureCancelled           ProviderFailureKind = "cancelled"
	FailureSourceInvalid       ProviderFailureKind = "source-invalid"
)

func (value ProviderFailureKind) IsValid() bool {
	return oneOf(value, FailureAuthentication, FailureRequestLimit, FailureProvider, FailureCancelled, FailureSourceInvalid)
}

func (value ProviderFailureKind) String() string {
	if !value.IsValid() {
		return string(UnknownProviderFailureKind)
	}
	return string(value)
}

func NewProviderFailureKind(input string) (ProviderFailureKind, bool) {
	return parseEnum(input, UnknownProviderFailureKind, FailureAuthentication, FailureRequestLimit, FailureProvider, FailureCancelled, FailureSourceInvalid)
}

type RetrievalStatus string

const (
	UnknownRetrievalStatus     RetrievalStatus = ""
	RetrievalStatusComplete    RetrievalStatus = "complete"
	RetrievalStatusIncomplete  RetrievalStatus = "incomplete"
	RetrievalStatusInterrupted RetrievalStatus = "interrupted"
)

func (value RetrievalStatus) IsValid() bool {
	return oneOf(value, RetrievalStatusComplete, RetrievalStatusIncomplete, RetrievalStatusInterrupted)
}

func (value RetrievalStatus) String() string {
	if !value.IsValid() {
		return string(UnknownRetrievalStatus)
	}
	return string(value)
}

func NewRetrievalStatus(input string) (RetrievalStatus, bool) {
	return parseEnum(input, UnknownRetrievalStatus, RetrievalStatusComplete, RetrievalStatusIncomplete, RetrievalStatusInterrupted)
}

type FileCategory string

const (
	UnknownFileCategory  FileCategory = ""
	FileCategoryCode     FileCategory = "code"
	FileCategoryConfig   FileCategory = "config"
	FileCategoryDocs     FileCategory = "docs"
	FileCategoryData     FileCategory = "data"
	FileCategoryAudio    FileCategory = "audio"
	FileCategoryVideo    FileCategory = "video"
	FileCategoryArchives FileCategory = "archives"
	FileCategoryLogs     FileCategory = "logs"
	FileCategoryTemp     FileCategory = "temp"
	FileCategoryUnknown  FileCategory = "unknown"
)

func (value FileCategory) IsValid() bool {
	return oneOf(value, FileCategoryCode, FileCategoryConfig, FileCategoryDocs, FileCategoryData, FileCategoryAudio, FileCategoryVideo, FileCategoryArchives, FileCategoryLogs, FileCategoryTemp, FileCategoryUnknown)
}

func (value FileCategory) String() string {
	if !value.IsValid() {
		return string(UnknownFileCategory)
	}
	return string(value)
}

func NewFileCategory(input string) (FileCategory, bool) {
	return parseEnum(input, UnknownFileCategory, FileCategoryCode, FileCategoryConfig, FileCategoryDocs, FileCategoryData, FileCategoryAudio, FileCategoryVideo, FileCategoryArchives, FileCategoryLogs, FileCategoryTemp, FileCategoryUnknown)
}

type FileReason string

const (
	UnknownFileReason          FileReason = ""
	FileReasonOutsideRoot      FileReason = "outside_root"
	FileReasonProtected        FileReason = "protected"
	FileReasonHidden           FileReason = "hidden"
	FileReasonSensitiveName    FileReason = "sensitive_name"
	FileReasonSymlink          FileReason = "symlink"
	FileReasonSpecialFile      FileReason = "special_file"
	FileReasonDependency       FileReason = "dependency"
	FileReasonIgnored          FileReason = "ignored"
	FileReasonExcludePattern   FileReason = "exclude_pattern"
	FileReasonNotDirectory     FileReason = "not_directory"
	FileReasonMissing          FileReason = "missing"
	FileReasonNotFile          FileReason = "not_file"
	FileReasonBinary           FileReason = "binary"
	FileReasonInvalidUTF8      FileReason = "invalid_utf8"
	FileReasonSensitiveContent FileReason = "sensitive_content"
	FileReasonFileTypeCode     FileReason = "file_type:code"
	FileReasonFileTypeConfig   FileReason = "file_type:config"
	FileReasonFileTypeDocs     FileReason = "file_type:docs"
	FileReasonFileTypeData     FileReason = "file_type:data"
	FileReasonFileTypeAudio    FileReason = "file_type:audio"
	FileReasonFileTypeVideo    FileReason = "file_type:video"
	FileReasonFileTypeArchives FileReason = "file_type:archives"
	FileReasonFileTypeLogs     FileReason = "file_type:logs"
	FileReasonFileTypeTemp     FileReason = "file_type:temp"
	FileReasonFileTypeUnknown  FileReason = "file_type:unknown"
)

func (value FileReason) IsValid() bool {
	return oneOf(value, FileReasonOutsideRoot, FileReasonProtected, FileReasonHidden, FileReasonSensitiveName, FileReasonSymlink, FileReasonSpecialFile, FileReasonDependency, FileReasonIgnored, FileReasonExcludePattern, FileReasonNotDirectory, FileReasonMissing, FileReasonNotFile, FileReasonBinary, FileReasonInvalidUTF8, FileReasonSensitiveContent, FileReasonFileTypeCode, FileReasonFileTypeConfig, FileReasonFileTypeDocs, FileReasonFileTypeData, FileReasonFileTypeAudio, FileReasonFileTypeVideo, FileReasonFileTypeArchives, FileReasonFileTypeLogs, FileReasonFileTypeTemp, FileReasonFileTypeUnknown)
}

func (value FileReason) String() string {
	if !value.IsValid() {
		return string(UnknownFileReason)
	}
	return string(value)
}

func NewFileReason(input string) (FileReason, bool) {
	return parseEnum(input, UnknownFileReason, FileReasonOutsideRoot, FileReasonProtected, FileReasonHidden, FileReasonSensitiveName, FileReasonSymlink, FileReasonSpecialFile, FileReasonDependency, FileReasonIgnored, FileReasonExcludePattern, FileReasonNotDirectory, FileReasonMissing, FileReasonNotFile, FileReasonBinary, FileReasonInvalidUTF8, FileReasonSensitiveContent, FileReasonFileTypeCode, FileReasonFileTypeConfig, FileReasonFileTypeDocs, FileReasonFileTypeData, FileReasonFileTypeAudio, FileReasonFileTypeVideo, FileReasonFileTypeArchives, FileReasonFileTypeLogs, FileReasonFileTypeTemp, FileReasonFileTypeUnknown)
}

type FileIssueKind string

const (
	UnknownFileIssueKind   FileIssueKind = ""
	FileIssueUnreadable    FileIssueKind = "unreadable"
	FileIssueChanged       FileIssueKind = "changed"
	FileIssueResourceLimit FileIssueKind = "resource_limit"
	FileIssueInterrupted   FileIssueKind = "interrupted"
	FileIssueDetector      FileIssueKind = "detector"
)

func (value FileIssueKind) IsValid() bool {
	return oneOf(value, FileIssueUnreadable, FileIssueChanged, FileIssueResourceLimit, FileIssueInterrupted, FileIssueDetector)
}

func (value FileIssueKind) String() string {
	if !value.IsValid() {
		return string(UnknownFileIssueKind)
	}
	return string(value)
}

func NewFileIssueKind(input string) (FileIssueKind, bool) {
	return parseEnum(input, UnknownFileIssueKind, FileIssueUnreadable, FileIssueChanged, FileIssueResourceLimit, FileIssueInterrupted, FileIssueDetector)
}

type LookupStatus string

const (
	UnknownLookupStatus  LookupStatus = ""
	LookupStatusFile     LookupStatus = "file"
	LookupStatusExcluded LookupStatus = "excluded"
)

func (value LookupStatus) IsValid() bool {
	return oneOf(value, LookupStatusFile, LookupStatusExcluded)
}

func (value LookupStatus) String() string {
	if !value.IsValid() {
		return string(UnknownLookupStatus)
	}
	return string(value)
}

func NewLookupStatus(input string) (LookupStatus, bool) {
	return parseEnum(input, UnknownLookupStatus, LookupStatusFile, LookupStatusExcluded)
}

type SnapshotStatus string

const (
	UnknownSnapshotStatus  SnapshotStatus = ""
	SnapshotStatusSnapshot SnapshotStatus = "snapshot"
	SnapshotStatusExcluded SnapshotStatus = "excluded"
	SnapshotStatusIssue    SnapshotStatus = "issue"
)

func (value SnapshotStatus) IsValid() bool {
	return oneOf(value, SnapshotStatusSnapshot, SnapshotStatusExcluded, SnapshotStatusIssue)
}

func (value SnapshotStatus) String() string {
	if !value.IsValid() {
		return string(UnknownSnapshotStatus)
	}
	return string(value)
}

func NewSnapshotStatus(input string) (SnapshotStatus, bool) {
	return parseEnum(input, UnknownSnapshotStatus, SnapshotStatusSnapshot, SnapshotStatusExcluded, SnapshotStatusIssue)
}

type NavigationKind string

const (
	UnknownNavigationKind   NavigationKind = ""
	NavigationKindFile      NavigationKind = "file"
	NavigationKindDirectory NavigationKind = "directory"
)

func (value NavigationKind) IsValid() bool {
	return oneOf(value, NavigationKindFile, NavigationKindDirectory)
}

func (value NavigationKind) String() string {
	if !value.IsValid() {
		return string(UnknownNavigationKind)
	}
	return string(value)
}

func NewNavigationKind(input string) (NavigationKind, bool) {
	return parseEnum(input, UnknownNavigationKind, NavigationKindFile, NavigationKindDirectory)
}

type AssessmentRole string

const (
	UnknownAssessmentRole        AssessmentRole = ""
	AssessmentRoleImplementation AssessmentRole = "implementation"
	AssessmentRoleCaller         AssessmentRole = "caller"
	AssessmentRoleTest           AssessmentRole = "test"
	AssessmentRoleFixture        AssessmentRole = "fixture"
	AssessmentRoleHelper         AssessmentRole = "helper"
)

func (value AssessmentRole) IsValid() bool {
	return oneOf(value, AssessmentRoleImplementation, AssessmentRoleCaller, AssessmentRoleTest, AssessmentRoleFixture, AssessmentRoleHelper)
}

func (value AssessmentRole) String() string {
	if !value.IsValid() {
		return string(UnknownAssessmentRole)
	}
	return string(value)
}

func NewAssessmentRole(input string) (AssessmentRole, bool) {
	return parseEnum(input, UnknownAssessmentRole, AssessmentRoleImplementation, AssessmentRoleCaller, AssessmentRoleTest, AssessmentRoleFixture, AssessmentRoleHelper)
}

type RetrievalIssueCode string

const (
	UnknownRetrievalIssueCode      RetrievalIssueCode = ""
	IssueCodeAuthentication        RetrievalIssueCode = "authentication"
	IssueCodeRequestLimit          RetrievalIssueCode = "request-limit"
	IssueCodeProvider              RetrievalIssueCode = "provider"
	IssueCodeCancelled             RetrievalIssueCode = "cancelled"
	IssueCodeInterrupted           RetrievalIssueCode = "interrupted"
	IssueCodeUnreadable            RetrievalIssueCode = "unreadable"
	IssueCodeChanged               RetrievalIssueCode = "changed"
	IssueCodeResourceLimit         RetrievalIssueCode = "resource_limit"
	IssueCodeDetector              RetrievalIssueCode = "detector"
	IssueCodeRequestSize           RetrievalIssueCode = "request-size"
	IssueCodeLowConfidenceBudget   RetrievalIssueCode = "low_confidence_budget"
	IssueCodeSourceInspectionLimit RetrievalIssueCode = "source_inspection_limit"
)

func (value RetrievalIssueCode) IsValid() bool {
	return oneOf(value, IssueCodeAuthentication, IssueCodeRequestLimit, IssueCodeProvider, IssueCodeCancelled, IssueCodeInterrupted, IssueCodeUnreadable, IssueCodeChanged, IssueCodeResourceLimit, IssueCodeDetector, IssueCodeRequestSize, IssueCodeLowConfidenceBudget, IssueCodeSourceInspectionLimit)
}

func (value RetrievalIssueCode) String() string {
	if !value.IsValid() {
		return string(UnknownRetrievalIssueCode)
	}
	return string(value)
}

func NewRetrievalIssueCode(input string) (RetrievalIssueCode, bool) {
	return parseEnum(input, UnknownRetrievalIssueCode, IssueCodeAuthentication, IssueCodeRequestLimit, IssueCodeProvider, IssueCodeCancelled, IssueCodeInterrupted, IssueCodeUnreadable, IssueCodeChanged, IssueCodeResourceLimit, IssueCodeDetector, IssueCodeRequestSize, IssueCodeLowConfidenceBudget, IssueCodeSourceInspectionLimit)
}
