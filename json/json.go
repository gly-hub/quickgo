// Package json provides a high-performance JSON encoding/decoding wrapper
// using json-iterator for better performance than standard library.
package json

import (
	jsoniter "github.com/json-iterator/go"
)

// json-iterator 配置
var (
	// JSON 使用与标准库完全兼容的配置
	JSON = jsoniter.ConfigCompatibleWithStandardLibrary

	// JSONFast 使用更快但稍有不同的配置
	// - 不对 map key 排序
	// - 不转义 HTML
	JSONFast = jsoniter.ConfigFastest
)

// Marshal 序列化为 JSON（与 encoding/json.Marshal 兼容）
func Marshal(v interface{}) ([]byte, error) {
	return JSON.Marshal(v)
}

// MarshalIndent 序列化为格式化的 JSON
func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return JSON.MarshalIndent(v, prefix, indent)
}

// Unmarshal 从 JSON 反序列化（与 encoding/json.Unmarshal 兼容）
func Unmarshal(data []byte, v interface{}) error {
	return JSON.Unmarshal(data, v)
}

// MarshalToString 序列化为 JSON 字符串
func MarshalToString(v interface{}) (string, error) {
	return JSON.MarshalToString(v)
}

// UnmarshalFromString 从 JSON 字符串反序列化
func UnmarshalFromString(str string, v interface{}) error {
	return JSON.UnmarshalFromString(str, v)
}

// MarshalFast 使用更快的配置序列化
func MarshalFast(v interface{}) ([]byte, error) {
	return JSONFast.Marshal(v)
}

// UnmarshalFast 使用更快的配置反序列化
func UnmarshalFast(data []byte, v interface{}) error {
	return JSONFast.Unmarshal(data, v)
}

// Valid 检查是否为有效的 JSON
func Valid(data []byte) bool {
	return JSON.Valid(data)
}

// NewEncoder 创建 JSON 编码器
func NewEncoder(w interface {
	Write(p []byte) (n int, err error)
}) *jsoniter.Encoder {
	return JSON.NewEncoder(w)
}

// NewDecoder 创建 JSON 解码器
func NewDecoder(r interface {
	Read(p []byte) (n int, err error)
}) *jsoniter.Decoder {
	return JSON.NewDecoder(r)
}

// Get 从 JSON 数据中获取指定路径的值
// path 可以是 "key", "key.subkey", 或数字索引
func Get(data []byte, path ...interface{}) jsoniter.Any {
	return jsoniter.Get(data, path...)
}

// RawMessage 原始 JSON 消息类型（与 encoding/json.RawMessage 兼容）
type RawMessage = jsoniter.RawMessage
