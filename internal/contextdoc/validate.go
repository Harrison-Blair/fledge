package contextdoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var groupIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var timeType = reflect.TypeFor[time.Time]()

const (
	MaxRequestFiles = 50
	MaxRequestBytes = int64(256000)
)

// ValidateAnalyzerRequest validates one exact analyzer request JSON value.
// It is the in-memory validation seam used by the daemon before a managed
// context request is journaled or delivered.
func ValidateAnalyzerRequest(data []byte) error {
	var request AnalyzerRequest
	if err := decodeExact(data, &request); err != nil {
		return err
	}
	return validateRequest(request)
}

// ValidateComposedAnalyzerRequest validates one exact analyzer request JSON
// value and additionally requires both instruction fields to be nonblank. The
// daemon uses it so an instruction-less request can never reach an analyzer.
func ValidateComposedAnalyzerRequest(data []byte) error {
	var request AnalyzerRequest
	if err := decodeExact(data, &request); err != nil {
		return err
	}
	if err := validateRequest(request); err != nil {
		return err
	}
	if strings.TrimSpace(request.InstructionsBefore) == "" {
		return errors.New("instructions_before must be nonempty; compose the request with \"fledge context compose analyzer-request\"")
	}
	if strings.TrimSpace(request.InstructionsAfter) == "" {
		return errors.New("instructions_after must be nonempty; compose the request with \"fledge context compose analyzer-request\"")
	}
	return nil
}

// ValidateAnalyzerRequestFile validates one exact analyzer request JSON file.
func ValidateAnalyzerRequestFile(name string) error {
	_, err := LoadAnalyzerRequest(name)
	return err
}

// LoadAnalyzerRequest decodes and validates one exact analyzer request.
func LoadAnalyzerRequest(name string) (AnalyzerRequest, error) {
	var request AnalyzerRequest
	if err := decodeExactFile(name, &request); err != nil {
		return request, err
	}
	if err := validateRequest(request); err != nil {
		return request, fmt.Errorf("%s: %w", name, err)
	}
	return request, nil
}

// ValidateAnalyzerReplyFile validates one exact analyzer reply. It correlates
// the group and assigned-file references against requestName; safe internal
// dependency paths may refer outside the request.
func ValidateAnalyzerReplyFile(name, requestName string) error {
	request, err := LoadAnalyzerRequest(requestName)
	if err != nil {
		return err
	}
	_, err = loadAnalyzerReply(name, request)
	return err
}

// ValidateAnalyzerReply validates one exact analyzer reply JSON value and
// correlates it with one exact analyzer request JSON value. It is the
// in-memory validation seam used by the daemon before a managed context reply
// is journaled or delivered.
func ValidateAnalyzerReply(data, requestData []byte) error {
	var request AnalyzerRequest
	if err := decodeExact(requestData, &request); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	if err := validateRequest(request); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	reply, err := decodeAnalyzerReply(data)
	if err != nil {
		return err
	}
	return validateReply(reply, request)
}

func loadAnalyzerReply(name string, request AnalyzerRequest) (AnalyzerReply, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return AnalyzerReply{}, err
	}
	reply, err := decodeAnalyzerReply(data)
	if err != nil {
		return AnalyzerReply{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := validateReply(reply, request); err != nil {
		return AnalyzerReply{}, fmt.Errorf("%s: %w", name, err)
	}
	return reply, nil
}

func decodeAnalyzerReply(data []byte) (AnalyzerReply, error) {
	if err := rejectDuplicateObjectKeys(data); err != nil {
		return AnalyzerReply{}, err
	}
	var header struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return AnalyzerReply{}, err
	}

	switch header.Status {
	case "ok":
		var wire AnalyzerSuccessReply
		if err := decodeExact(data, &wire); err != nil {
			return AnalyzerReply{}, err
		}
		return AnalyzerReply{
			SchemaVersion:    wire.SchemaVersion,
			Status:           wire.Status,
			GroupID:          wire.GroupID,
			SubsystemSummary: wire.SubsystemSummary,
			EntryPoints:      wire.EntryPoints,
			KeySymbols:       wire.KeySymbols,
			Dependencies:     wire.Dependencies,
			DataFlows:        wire.DataFlows,
			Invariants:       wire.Invariants,
			Tests:            wire.Tests,
			Files:            wire.Files,
		}, nil
	case "error":
		var wire AnalyzerErrorReply
		if err := decodeExact(data, &wire); err != nil {
			return AnalyzerReply{}, err
		}
		return AnalyzerReply{
			SchemaVersion: wire.SchemaVersion,
			Status:        wire.Status,
			GroupID:       wire.GroupID,
			Errors:        wire.Errors,
		}, nil
	default:
		return AnalyzerReply{}, fmt.Errorf("status must be %q or %q", "ok", "error")
	}
}

func validateRequest(request AnalyzerRequest) error {
	if request.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %d", SchemaVersion)
	}
	if !groupIDPattern.MatchString(request.GroupID) {
		return errors.New("group_id must be nonempty kebab-case")
	}
	if strings.TrimSpace(request.Purpose) == "" {
		return errors.New("purpose must be nonempty")
	}
	if request.TotalSize < 0 {
		return errors.New("total_size must be nonnegative")
	}
	if len(request.Files) == 0 {
		return errors.New("files must be nonempty")
	}
	total, _, err := validateFiles(request.Files)
	if err != nil {
		return err
	}
	if request.TotalSize != total {
		return fmt.Errorf("total_size is %d, want %d", request.TotalSize, total)
	}
	if len(request.Files) > MaxRequestFiles {
		return fmt.Errorf("files contains %d entries, maximum is %d", len(request.Files), MaxRequestFiles)
	}
	if request.TotalSize > MaxRequestBytes && len(request.Files) != 1 {
		return fmt.Errorf("total_size %d exceeds %d bytes and is not an oversized singleton", request.TotalSize, MaxRequestBytes)
	}
	return nil
}

func validateReply(reply AnalyzerReply, request AnalyzerRequest) error {
	if reply.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %d", SchemaVersion)
	}
	if reply.GroupID != request.GroupID {
		return fmt.Errorf("group_id %q does not match request %q", reply.GroupID, request.GroupID)
	}

	assigned := make(map[string]bool, len(request.Files))
	for _, file := range request.Files {
		assigned[file.Path] = true
	}
	if reply.Status == "error" {
		if len(reply.Errors) == 0 {
			return errors.New("error reply must contain at least one error")
		}
		for i, item := range reply.Errors {
			if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Message) == "" {
				return fmt.Errorf("errors[%d] code and message must be nonempty", i)
			}
			if item.Path != "" && !assigned[item.Path] {
				return fmt.Errorf("errors[%d] path %q is not assigned by the request", i, item.Path)
			}
		}
		return nil
	}

	if strings.TrimSpace(reply.SubsystemSummary) == "" {
		return errors.New("subsystem_summary must be nonempty")
	}
	if reply.EntryPoints == nil || reply.KeySymbols == nil ||
		reply.Dependencies.Internal == nil || reply.Dependencies.External == nil ||
		reply.DataFlows == nil || reply.Invariants == nil || reply.Tests == nil ||
		reply.Files == nil {
		return errors.New("all success reply arrays must be present")
	}

	contentKinds := make(map[string]string, len(reply.Files))
	for i, file := range reply.Files {
		if !assigned[file.Path] {
			return fmt.Errorf("files[%d] path %q is not assigned by the request", i, file.Path)
		}
		if _, exists := contentKinds[file.Path]; exists {
			return fmt.Errorf("files contains duplicate path %q", file.Path)
		}
		if file.ContentKind != "text" && file.ContentKind != "non-text" {
			return fmt.Errorf("files[%d] content_kind must be %q or %q", i, "text", "non-text")
		}
		if strings.TrimSpace(file.Summary) == "" {
			return fmt.Errorf("files[%d] summary must be nonempty", i)
		}
		contentKinds[file.Path] = file.ContentKind
	}
	for file := range assigned {
		if _, ok := contentKinds[file]; !ok {
			return fmt.Errorf("files is missing assigned path %q", file)
		}
	}

	validateTextPath := func(label, file string) error {
		kind, ok := contentKinds[file]
		if !ok {
			return fmt.Errorf("%s path %q is not assigned by the request", label, file)
		}
		if kind != "text" {
			return fmt.Errorf("%s path %q refers to a non-text file", label, file)
		}
		return nil
	}
	for i, item := range reply.EntryPoints {
		if strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("entry_points[%d] description must be nonempty", i)
		}
		if err := validateTextPath(fmt.Sprintf("entry_points[%d]", i), item.Path); err != nil {
			return err
		}
	}
	for i, item := range reply.KeySymbols {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Kind) == "" ||
			strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("key_symbols[%d] name, kind, and description must be nonempty", i)
		}
		if err := validateTextPath(fmt.Sprintf("key_symbols[%d]", i), item.Path); err != nil {
			return err
		}
	}
	for i, item := range reply.Dependencies.Internal {
		if strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("dependencies.internal[%d] description must be nonempty", i)
		}
		if !validRelativePath(item.Path) {
			return fmt.Errorf("dependencies.internal[%d] path %q is not a safe normalized relative path", i, item.Path)
		}
	}
	for i, item := range reply.Dependencies.External {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("dependencies.external[%d] name and description must be nonempty", i)
		}
	}
	for i, item := range reply.DataFlows {
		if strings.TrimSpace(item.From) == "" || strings.TrimSpace(item.To) == "" ||
			strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("data_flows[%d] from, to, and description must be nonempty", i)
		}
	}
	for i, item := range reply.Invariants {
		if strings.TrimSpace(item) == "" {
			return fmt.Errorf("invariants[%d] must be nonempty", i)
		}
	}
	for i, item := range reply.Tests {
		if strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("tests[%d] description must be nonempty", i)
		}
		if err := validateTextPath(fmt.Sprintf("tests[%d]", i), item.Path); err != nil {
			return err
		}
	}
	return nil
}

func validateFiles(files []File) (int64, map[string]int64, error) {
	total := int64(0)
	seen := make(map[string]int64, len(files))
	for i, file := range files {
		if !validRelativePath(file.Path) {
			return 0, nil, fmt.Errorf("files[%d] path %q is not a safe normalized relative path", i, file.Path)
		}
		if file.Size < 0 {
			return 0, nil, fmt.Errorf("files[%d] size must be nonnegative", i)
		}
		if _, ok := seen[file.Path]; ok {
			return 0, nil, fmt.Errorf("files contains duplicate path %q", file.Path)
		}
		if file.Size > math.MaxInt64-total {
			return 0, nil, errors.New("file sizes overflow total_size")
		}
		seen[file.Path] = file.Size
		total += file.Size
	}
	return total, seen, nil
}

func validRelativePath(name string) bool {
	return fs.ValidPath(name) && !strings.Contains(name, `\`) && path.Clean(name) == name
}

func decodeExactFile(name string, out any) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	if err := decodeExact(data, out); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func decodeExact(data []byte, out any) error {
	if err := rejectDuplicateObjectKeys(data); err != nil {
		return err
	}
	var shape any
	shapeDecoder := json.NewDecoder(bytes.NewReader(data))
	shapeDecoder.UseNumber()
	if err := shapeDecoder.Decode(&shape); err != nil {
		return err
	}
	if err := requireExactShape(shape, reflect.TypeOf(out)); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := requireEOF(decoder); err != nil {
		return err
	}
	return nil
}

func requireExactShape(value any, typ reflect.Type) error {
	if value == nil {
		return fmt.Errorf("expected non-null JSON value for %s", typ)
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == timeType {
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected JSON object for %s", typ)
		}
		fields := make(map[string]reflect.StructField)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tag := field.Tag.Get("json")
			name, options, _ := strings.Cut(tag, ",")
			if name == "" {
				name = field.Name
			}
			if name == "-" {
				continue
			}
			fields[name] = field
			if !strings.Contains(options, "omitempty") {
				if _, exists := object[name]; !exists {
					return fmt.Errorf("missing required field %q", name)
				}
			}
		}
		for name, child := range object {
			field, exists := fields[name]
			if !exists {
				return fmt.Errorf("unknown field %q", name)
			}
			if err := requireExactShape(child, field.Type); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	case reflect.Slice, reflect.Array:
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("expected JSON array for %s", typ)
		}
		for i, child := range array {
			if err := requireExactShape(child, typ.Elem()); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
	}
	return nil
}

func rejectDuplicateObjectKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walkValue func() error
	walkValue = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := token.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if seen[name] {
					return fmt.Errorf("duplicate object field %q", name)
				}
				seen[name] = true
				if err := walkValue(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walkValue(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected delimiter %q", delim)
		}
	}
	if err := walkValue(); err != nil {
		return err
	}
	return requireEOF(decoder)
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}
