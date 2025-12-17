package quickgo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// 支持的配置后缀
const (
	ConfigFormatJSON = "json"
	ConfigFormatYAML = "yaml"
	ConfigFormatTOML = "toml"
	ConfigFormatINI  = "ini"
)

// 环境类型
const (
	EnvLocal      = "local"      // 本地环境
	EnvDevelop    = "develop"    // 测试环境
	EnvRelease    = "release"    // 预发布环境
	EnvProduction = "production" // 生产环境
)

// 环境变量名
const (
	EnvVarName = "DANDELION_ENV"
)

// 支持的配置格式
var supportedFormats = []string{ConfigFormatJSON, ConfigFormatYAML, ConfigFormatTOML, ConfigFormatINI}

// ConfigLoader 配置加载器
type ConfigLoader struct {
	env          string
	configPath   string
	configName   string
	configFormat string
	viper        *viper.Viper
}

// NewConfigLoader 创建配置加载器
// env: 环境名称（local, develop, release, production）
// configPath: 配置文件目录路径（可选，为空时自动查找）
func NewConfigLoader(env string, configPath ...string) (*ConfigLoader, error) {
	// 验证环境
	if !isValidEnv(env) {
		return nil, fmt.Errorf("unsupported environment: %s, supported: %v", env, []string{EnvLocal, EnvDevelop, EnvRelease, EnvProduction})
	}

	// 获取系统环境变量（优先级更高）
	if osEnv := os.Getenv(EnvVarName); osEnv != "" {
		if isValidEnv(osEnv) {
		env = osEnv
	}
	}

	loader := &ConfigLoader{
		env:        env,
		configName: fmt.Sprintf("configs_%s", env),
	}

	// 确定配置路径
	if len(configPath) > 0 && configPath[0] != "" {
		loader.configPath = configPath[0]
	} else {
		// 自动查找配置目录
		path, err := findConfigDir()
		if err != nil {
			return nil, fmt.Errorf("failed to find config directory: %w", err)
		}
		loader.configPath = path
	}

	// 检测配置文件格式
	format, err := detectConfigFormat(loader.configPath, loader.configName)
	if err != nil {
		return nil, fmt.Errorf("failed to detect config format: %w", err)
	}
	loader.configFormat = format

	// 初始化 viper
	loader.viper = viper.New()
	loader.viper.AddConfigPath(loader.configPath)
	loader.viper.SetConfigName(loader.configName)
	loader.viper.SetConfigType(loader.configFormat)

	// 读取配置文件
	if err := loader.viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return loader, nil
}

// Load 加载配置到指定的结构体
// configs: 配置结构体指针，可以传入多个
// 注意：会根据配置文件格式自动选择对应的标签（yaml/toml/json）
// 例如：如果配置文件是 YAML，会使用 yaml 标签；如果是 TOML，会使用 toml 标签
func (l *ConfigLoader) Load(configs ...interface{}) error {
	if len(configs) == 0 {
		return errors.New("no config structs provided")
	}

	// 根据配置文件格式确定使用的标签名
	tagName := l.getTagNameForFormat()

	for i, cfg := range configs {
		if cfg == nil {
			return fmt.Errorf("config[%d] is nil", i)
		}

		// 配置 mapstructure 使用对应格式的标签
		// 这样结构体的标签就能正确匹配配置文件的键名
		decoderConfig := &mapstructure.DecoderConfig{
			Metadata:         nil,
			Result:           cfg,
			WeaklyTypedInput: true,
			TagName:          tagName, // 根据配置文件格式选择标签
		}

		decoder, err := mapstructure.NewDecoder(decoderConfig)
		if err != nil {
			return fmt.Errorf("failed to create decoder for config[%d]: %w", i, err)
	}

		// 将 viper 的所有配置转换为 map
		configMap := l.viper.AllSettings()
		if err := decoder.Decode(configMap); err != nil {
			return fmt.Errorf("failed to unmarshal config[%d]: %w", i, err)
		}
	}

	return nil
}

// getTagNameForFormat 根据配置文件格式返回对应的标签名
func (l *ConfigLoader) getTagNameForFormat() string {
	switch l.configFormat {
	case ConfigFormatYAML:
		return "yaml"
	case ConfigFormatTOML:
		return "toml"
	case ConfigFormatJSON:
		return "json"
	case ConfigFormatINI:
		// INI 格式通常使用 mapstructure 标签，如果没有则回退到字段名
		return "mapstructure"
	default:
		// 默认使用 yaml（向后兼容）
		return "yaml"
	}
}

// LoadKey 加载指定键的配置到结构体
// key: 配置文件的键名（例如："app", "logger", "grpcServer"）
// cfg: 配置结构体指针
func (l *ConfigLoader) LoadKey(key string, cfg interface{}) error {
	if cfg == nil {
		return errors.New("config struct is nil")
	}

	// 根据配置文件格式确定使用的标签名
	tagName := l.getTagNameForFormat()

	// 配置 mapstructure 使用对应格式的标签
	decoderConfig := &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           cfg,
		WeaklyTypedInput: true,
		TagName:          tagName,
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	// 获取指定键的配置值
	configValue := l.viper.Get(key)
	if configValue == nil {
		return fmt.Errorf("config key %s not found", key)
	}

	// 直接解码配置值
	if err := decoder.Decode(configValue); err != nil {
		return fmt.Errorf("failed to unmarshal key %s: %w", key, err)
	}

	return nil
}

// GetViper 获取底层 viper 实例（用于高级用法）
func (l *ConfigLoader) GetViper() *viper.Viper {
	return l.viper
}

// GetEnv 获取当前环境
func (l *ConfigLoader) GetEnv() string {
	return l.env
}

// GetConfigPath 获取配置路径
func (l *ConfigLoader) GetConfigPath() string {
	return l.configPath
}

// GetConfigName 获取配置文件名（不含扩展名）
func (l *ConfigLoader) GetConfigName() string {
	return l.configName
}

// GetConfigFormat 获取配置文件格式
func (l *ConfigLoader) GetConfigFormat() string {
	return l.configFormat
}

// ==================== 全局便捷函数（向后兼容） ====================

var (
	globalLoader *ConfigLoader
	globalEnv    string
)

// InitConfig 初始化全局配置加载器（向后兼容）
// env: 环境名称
// configPath: 配置文件目录路径（可选）
// 注意：如果返回错误，会 panic（保持向后兼容）
func InitConfig(env string, configPath ...string) {
	loader, err := NewConfigLoader(env, configPath...)
		if err != nil {
			panic(err)
		}
	globalLoader = loader
	globalEnv = env
}

// LoadCustomConfig 使用全局配置加载器加载配置（向后兼容）
// configs: 配置结构体指针，可以传入多个
// 注意：如果返回错误，会 panic（保持向后兼容）
//
// 使用说明：
// 1. 会根据配置文件格式自动选择对应的标签（yaml/toml/json）
// 2. 如果配置文件是 YAML，会使用 yaml 标签；如果是 TOML，会使用 toml 标签
// 3. 如果结构体有对应格式的标签（如 yaml:"app" 或 toml:"app"），会自动匹配配置文件的键
// 4. 如果没有对应格式的标签，会尝试使用结构体字段名（小写）匹配
// 5. 推荐方式：使用 LoadCustomConfigKey 显式指定键名
func LoadCustomConfig(configs ...interface{}) {
	if globalLoader == nil {
		panic("config not initialized, call InitConfig first")
	}
	if err := globalLoader.Load(configs...); err != nil {
		panic(err)
	}
}

// LoadCustomConfigKey 使用全局配置加载器加载指定键的配置（推荐）
// key: 配置文件的键名（例如："app", "logger", "grpcServer"）
// cfg: 配置结构体指针
func LoadCustomConfigKey(key string, cfg interface{}) {
	if globalLoader == nil {
		panic("config not initialized, call InitConfig first")
	}
	if err := globalLoader.LoadKey(key, cfg); err != nil {
			panic(err)
		}
	}

// GetEnv 获取全局环境（向后兼容）
func GetEnv() string {
	if globalLoader != nil {
		return globalLoader.GetEnv()
	}
	return globalEnv
}

// ==================== 内部辅助函数 ====================

// isValidEnv 验证环境是否有效
func isValidEnv(env string) bool {
	return env == EnvLocal || env == EnvDevelop || env == EnvRelease || env == EnvProduction
}

// findConfigDir 查找配置目录
// 从当前工作目录向上查找，直到找到包含 "config" 目录的路径
func findConfigDir() (string, error) {
	wd, err := os.Getwd()
		if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
		}

	current := wd
	for {
		configPath := filepath.Join(current, "config")
		if info, err := os.Stat(configPath); err == nil && info.IsDir() {
			return configPath, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			// 已到达根目录
			break
		}
		current = parent
	}

	return "", errors.New("config directory not found, please specify config path explicitly")
	}

// detectConfigFormat 检测配置文件格式
func detectConfigFormat(configPath, configName string) (string, error) {
	entries, err := os.ReadDir(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))

		// 检查是否是支持的格式
		if !contains(supportedFormats, ext) {
			continue
		}

		// 检查文件名是否匹配（不含扩展名）
		nameWithoutExt := strings.TrimSuffix(name, filepath.Ext(name))
		if nameWithoutExt == configName {
			return ext, nil
		}
	}

	return "", fmt.Errorf("config file not found: %s (supported formats: %v)", configName, supportedFormats)
}

// contains 检查切片是否包含指定元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
