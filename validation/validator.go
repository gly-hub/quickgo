package validation

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ValidationError 验证错误
type ValidationError struct {
	Field   string // 字段名
	Tag     string // 验证标签
	Value   interface{} // 字段值
	Message string // 错误信息
}

func (e *ValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("field '%s' failed '%s' validation", e.Field, e.Tag)
}

// ValidationErrors 验证错误列表
type ValidationErrors []*ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validator 配置验证器
type Validator struct {
	tagName string
}

// NewValidator 创建验证器
func NewValidator() *Validator {
	return &Validator{
		tagName: "validate",
	}
}

// Validate 验证结构体
func (v *Validator) Validate(cfg interface{}) error {
	return v.validateValue(reflect.ValueOf(cfg), "")
}

func (v *Validator) validateValue(val reflect.Value, prefix string) error {
	var errs ValidationErrors

	// 处理指针
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	// 只处理结构体
	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// 跳过非导出字段
		if !field.IsExported() {
			continue
		}

		fieldName := field.Name
		if prefix != "" {
			fieldName = prefix + "." + fieldName
		}

		// 获取验证标签
		tag := field.Tag.Get(v.tagName)
		if tag == "" {
			// 递归验证嵌套结构体
			if fieldVal.Kind() == reflect.Struct || (fieldVal.Kind() == reflect.Ptr && fieldVal.Type().Elem().Kind() == reflect.Struct) {
				if err := v.validateValue(fieldVal, fieldName); err != nil {
					if verrs, ok := err.(ValidationErrors); ok {
						errs = append(errs, verrs...)
					} else {
						return err
					}
				}
			}
			continue
		}

		// 解析并执行验证规则
		rules := strings.Split(tag, ",")
		for _, rule := range rules {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				continue
			}

			if err := v.validateRule(fieldName, fieldVal, rule); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (v *Validator) validateRule(fieldName string, val reflect.Value, rule string) *ValidationError {
	// 解析规则名和参数
	ruleName := rule
	ruleParam := ""
	if idx := strings.Index(rule, "="); idx > 0 {
		ruleName = rule[:idx]
		ruleParam = rule[idx+1:]
	}

	// 获取实际值
	actualVal := val
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			if ruleName == "required" {
				return &ValidationError{Field: fieldName, Tag: ruleName, Value: nil, Message: fmt.Sprintf("field '%s' is required", fieldName)}
			}
			return nil
		}
		actualVal = val.Elem()
	}

	switch ruleName {
	case "required":
		if !hasValue(actualVal) {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s' is required", fieldName)}
		}

	case "min":
		if err := validateMin(actualVal, ruleParam); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "max":
		if err := validateMax(actualVal, ruleParam); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "oneof":
		values := strings.Split(ruleParam, " ")
		if err := validateOneOf(actualVal, values); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "url":
		if err := validateURL(actualVal); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "hostname":
		if err := validateHostname(actualVal); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "ip":
		if err := validateIP(actualVal); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "port":
		if err := validatePort(actualVal); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "duration":
		if err := validateDuration(actualVal); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "regex":
		if err := validateRegex(actualVal, ruleParam); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "gt":
		if err := validateGT(actualVal, ruleParam); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "gte":
		if err := validateGTE(actualVal, ruleParam); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "lt":
		if err := validateLT(actualVal, ruleParam); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "lte":
		if err := validateLTE(actualVal, ruleParam); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "len":
		if err := validateLen(actualVal, ruleParam); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}

	case "filepath":
		if err := validateFilePath(actualVal); err != nil {
			return &ValidationError{Field: fieldName, Tag: ruleName, Value: actualVal.Interface(), Message: fmt.Sprintf("field '%s': %s", fieldName, err.Error())}
		}
	}

	return nil
}

func hasValue(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.String:
		return val.String() != ""
	case reflect.Slice, reflect.Map, reflect.Array:
		return val.Len() > 0
	case reflect.Bool:
		return true // bool 类型始终有值
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return val.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return val.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return val.Float() != 0
	case reflect.Ptr, reflect.Interface:
		return !val.IsNil()
	default:
		return !val.IsZero()
	}
}

func validateMin(val reflect.Value, param string) error {
	min, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return fmt.Errorf("invalid min parameter: %s", param)
	}

	switch val.Kind() {
	case reflect.String:
		if len(val.String()) < int(min) {
			return fmt.Errorf("length must be at least %d", int(min))
		}
	case reflect.Slice, reflect.Map, reflect.Array:
		if val.Len() < int(min) {
			return fmt.Errorf("length must be at least %d", int(min))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if float64(val.Int()) < min {
			return fmt.Errorf("value must be at least %v", min)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if float64(val.Uint()) < min {
			return fmt.Errorf("value must be at least %v", min)
		}
	case reflect.Float32, reflect.Float64:
		if val.Float() < min {
			return fmt.Errorf("value must be at least %v", min)
		}
	}
	return nil
}

func validateMax(val reflect.Value, param string) error {
	max, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return fmt.Errorf("invalid max parameter: %s", param)
	}

	switch val.Kind() {
	case reflect.String:
		if len(val.String()) > int(max) {
			return fmt.Errorf("length must be at most %d", int(max))
		}
	case reflect.Slice, reflect.Map, reflect.Array:
		if val.Len() > int(max) {
			return fmt.Errorf("length must be at most %d", int(max))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if float64(val.Int()) > max {
			return fmt.Errorf("value must be at most %v", max)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if float64(val.Uint()) > max {
			return fmt.Errorf("value must be at most %v", max)
		}
	case reflect.Float32, reflect.Float64:
		if val.Float() > max {
			return fmt.Errorf("value must be at most %v", max)
		}
	}
	return nil
}

func validateOneOf(val reflect.Value, values []string) error {
	var strVal string
	switch val.Kind() {
	case reflect.String:
		strVal = val.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		strVal = strconv.FormatInt(val.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		strVal = strconv.FormatUint(val.Uint(), 10)
	default:
		strVal = fmt.Sprintf("%v", val.Interface())
	}

	for _, v := range values {
		if strVal == v {
			return nil
		}
	}
	return fmt.Errorf("must be one of: %s", strings.Join(values, ", "))
}

func validateURL(val reflect.Value) error {
	if val.Kind() != reflect.String {
		return fmt.Errorf("must be a string")
	}
	s := val.String()
	if s == "" {
		return nil
	}
	_, err := url.ParseRequestURI(s)
	if err != nil {
		return fmt.Errorf("invalid URL format")
	}
	return nil
}

func validateHostname(val reflect.Value) error {
	if val.Kind() != reflect.String {
		return fmt.Errorf("must be a string")
	}
	s := val.String()
	if s == "" {
		return nil
	}
	// RFC 1123 hostname regex
	re := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	if !re.MatchString(s) {
		return fmt.Errorf("invalid hostname format")
	}
	return nil
}

func validateIP(val reflect.Value) error {
	if val.Kind() != reflect.String {
		return fmt.Errorf("must be a string")
	}
	s := val.String()
	if s == "" {
		return nil
	}
	if net.ParseIP(s) == nil {
		return fmt.Errorf("invalid IP address")
	}
	return nil
}

func validatePort(val reflect.Value) error {
	var port int64
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		port = val.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		port = int64(val.Uint())
	case reflect.String:
		p, err := strconv.ParseInt(val.String(), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid port number")
		}
		port = p
	default:
		return fmt.Errorf("must be a number or string")
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func validateDuration(val reflect.Value) error {
	if val.Kind() != reflect.String {
		return fmt.Errorf("must be a string")
	}
	s := val.String()
	if s == "" {
		return nil
	}
	_, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration format")
	}
	return nil
}

func validateRegex(val reflect.Value, pattern string) error {
	if val.Kind() != reflect.String {
		return fmt.Errorf("must be a string")
	}
	s := val.String()
	if s == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %s", pattern)
	}
	if !re.MatchString(s) {
		return fmt.Errorf("must match pattern: %s", pattern)
	}
	return nil
}

func validateGT(val reflect.Value, param string) error {
	threshold, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return fmt.Errorf("invalid gt parameter: %s", param)
	}

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if float64(val.Int()) <= threshold {
			return fmt.Errorf("must be greater than %v", threshold)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if float64(val.Uint()) <= threshold {
			return fmt.Errorf("must be greater than %v", threshold)
		}
	case reflect.Float32, reflect.Float64:
		if val.Float() <= threshold {
			return fmt.Errorf("must be greater than %v", threshold)
		}
	}
	return nil
}

func validateGTE(val reflect.Value, param string) error {
	threshold, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return fmt.Errorf("invalid gte parameter: %s", param)
	}

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if float64(val.Int()) < threshold {
			return fmt.Errorf("must be greater than or equal to %v", threshold)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if float64(val.Uint()) < threshold {
			return fmt.Errorf("must be greater than or equal to %v", threshold)
		}
	case reflect.Float32, reflect.Float64:
		if val.Float() < threshold {
			return fmt.Errorf("must be greater than or equal to %v", threshold)
		}
	}
	return nil
}

func validateLT(val reflect.Value, param string) error {
	threshold, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return fmt.Errorf("invalid lt parameter: %s", param)
	}

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if float64(val.Int()) >= threshold {
			return fmt.Errorf("must be less than %v", threshold)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if float64(val.Uint()) >= threshold {
			return fmt.Errorf("must be less than %v", threshold)
		}
	case reflect.Float32, reflect.Float64:
		if val.Float() >= threshold {
			return fmt.Errorf("must be less than %v", threshold)
		}
	}
	return nil
}

func validateLTE(val reflect.Value, param string) error {
	threshold, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return fmt.Errorf("invalid lte parameter: %s", param)
	}

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if float64(val.Int()) > threshold {
			return fmt.Errorf("must be less than or equal to %v", threshold)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if float64(val.Uint()) > threshold {
			return fmt.Errorf("must be less than or equal to %v", threshold)
		}
	case reflect.Float32, reflect.Float64:
		if val.Float() > threshold {
			return fmt.Errorf("must be less than or equal to %v", threshold)
		}
	}
	return nil
}

func validateLen(val reflect.Value, param string) error {
	length, err := strconv.Atoi(param)
	if err != nil {
		return fmt.Errorf("invalid len parameter: %s", param)
	}

	switch val.Kind() {
	case reflect.String:
		if len(val.String()) != length {
			return fmt.Errorf("length must be exactly %d", length)
		}
	case reflect.Slice, reflect.Map, reflect.Array:
		if val.Len() != length {
			return fmt.Errorf("length must be exactly %d", length)
		}
	}
	return nil
}

func validateFilePath(val reflect.Value) error {
	if val.Kind() != reflect.String {
		return fmt.Errorf("must be a string")
	}
	s := val.String()
	if s == "" {
		return nil
	}
	// 基本的文件路径验证
	if strings.ContainsAny(s, "\x00") {
		return fmt.Errorf("invalid file path")
	}
	return nil
}

// ValidateConfig 验证配置结构体（便捷函数）
func ValidateConfig(cfg interface{}) error {
	return NewValidator().Validate(cfg)
}
