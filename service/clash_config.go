package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gopkg.in/yaml.v3"
)

type ClashConfig struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Valid bool   `json:"valid"`
}

type ClashConfigFile struct {
	Rules []string               `yaml:"rules"`
	Other map[string]interface{} `yaml:",inline"`
}

// FindClashConfigs 查找系统中所有可能的 Clash Verge 配置文件
func FindClashConfigs() ([]ClashConfig, error) {
	var configs []ClashConfig
	var searchPaths []string

	switch runtime.GOOS {
	case "windows":
		// Windows: %USERPROFILE%\.config\clash-verge, %APPDATA%\clash-verge
		userProfile := os.Getenv("USERPROFILE")
		appData := os.Getenv("APPDATA")
		localAppData := os.Getenv("LOCALAPPDATA")

		searchPaths = []string{
			filepath.Join(userProfile, ".config", "clash-verge"),
			filepath.Join(appData, "clash-verge"),
			filepath.Join(localAppData, "clash-verge"),
			filepath.Join(userProfile, ".config", "mihomo"),
			filepath.Join(appData, "mihomo"),
		}
	case "darwin":
		// macOS: ~/Library/Application Support/clash-verge
		home := os.Getenv("HOME")
		searchPaths = []string{
			filepath.Join(home, "Library", "Application Support", "clash-verge"),
			filepath.Join(home, "Library", "Application Support", "mihomo"),
			filepath.Join(home, ".config", "clash-verge"),
			filepath.Join(home, ".config", "mihomo"),
		}
	case "linux":
		// Linux: ~/.config/clash-verge
		home := os.Getenv("HOME")
		searchPaths = []string{
			filepath.Join(home, ".config", "clash-verge"),
			filepath.Join(home, ".config", "mihomo"),
			filepath.Join(home, ".clash-verge"),
			filepath.Join(home, ".mihomo"),
		}
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	// 搜索配置文件
	for _, basePath := range searchPaths {
		if _, err := os.Stat(basePath); err != nil {
			continue
		}

		// 查找常见的配置文件名
		configFiles := []string{
			"config.yaml",
			"config.yml",
			"profiles/config.yaml",
			"profiles/config.yml",
		}

		for _, configFile := range configFiles {
			fullPath := filepath.Join(basePath, configFile)
			if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
				// 验证是否为有效的 Clash 配置
				valid := validateClashConfig(fullPath)
				configs = append(configs, ClashConfig{
					Path:  fullPath,
					Name:  filepath.Base(basePath) + "/" + configFile,
					Valid: valid,
				})
			}
		}

		// 查找 profiles 目录下的其他配置文件
		profilesDir := filepath.Join(basePath, "profiles")
		if entries, err := os.ReadDir(profilesDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
					fullPath := filepath.Join(profilesDir, entry.Name())
					valid := validateClashConfig(fullPath)
					configs = append(configs, ClashConfig{
						Path:  fullPath,
						Name:  filepath.Base(basePath) + "/profiles/" + entry.Name(),
						Valid: valid,
					})
				}
			}
		}
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no Clash Verge config files found")
	}

	return configs, nil
}

// validateClashConfig 验证是否为有效的 Clash 配置文件
func validateClashConfig(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return false
	}

	// 检查是否包含 Clash 配置的关键字段
	_, hasProxies := config["proxies"]
	_, hasRules := config["rules"]
	_, hasProxyGroups := config["proxy-groups"]

	return hasProxies || hasRules || hasProxyGroups
}

// AddDirectRule 向 Clash 配置文件添加直连规则
func AddDirectRule(configPath string, channelName string, baseURL string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var config ClashConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// 从 baseURL 提取域名
	domain := extractDomain(baseURL)
	if domain == "" {
		return fmt.Errorf("invalid baseURL: %s", baseURL)
	}

	// 生成规则注释和规则
	ruleComment := fmt.Sprintf("# new-api channel: %s", channelName)
	domainRule := fmt.Sprintf("DOMAIN-SUFFIX,%s,DIRECT", domain)

	// 检查规则是否已存在
	ruleExists := false
	for _, rule := range config.Rules {
		if strings.Contains(rule, domain) && strings.Contains(rule, "DIRECT") {
			ruleExists = true
			break
		}
	}

	if !ruleExists {
		// 在规则列表开头添加新规则（高优先级）
		newRules := []string{ruleComment, domainRule}
		config.Rules = append(newRules, config.Rules...)
	}

	// 备份原文件
	backupPath := configPath + ".backup"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		common.SysLog(fmt.Sprintf("failed to create backup: %v", err))
	}

	// 序列化并写回配置文件
	output, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	common.SysLog(fmt.Sprintf("Added direct rule for %s to %s", domain, configPath))
	return nil
}

// RemoveDirectRule 从 Clash 配置文件移除直连规则
func RemoveDirectRule(configPath string, baseURL string) error {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var config ClashConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// 从 baseURL 提取域名
	domain := extractDomain(baseURL)
	if domain == "" {
		return fmt.Errorf("invalid baseURL: %s", baseURL)
	}

	// 过滤掉包含该域名的规则及其注释
	var newRules []string
	skipNext := false
	for i, rule := range config.Rules {
		// 如果当前规则包含目标域名，跳过
		if strings.Contains(rule, domain) && strings.Contains(rule, "DIRECT") {
			// 如果前一条是注释，也要移除
			if len(newRules) > 0 && strings.HasPrefix(newRules[len(newRules)-1], "#") {
				newRules = newRules[:len(newRules)-1]
			}
			continue
		}

		// 如果是 new-api 相关的注释，检查下一条规则
		if strings.HasPrefix(rule, "# new-api channel:") {
			// 检查下一条规则是否包含目标域名
			if i+1 < len(config.Rules) {
				nextRule := config.Rules[i+1]
				if strings.Contains(nextRule, domain) && strings.Contains(nextRule, "DIRECT") {
					skipNext = true
					continue
				}
			}
		}

		if skipNext {
			skipNext = false
			continue
		}

		newRules = append(newRules, rule)
	}

	config.Rules = newRules

	// 备份原文件
	backupPath := configPath + ".backup"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		common.SysLog(fmt.Sprintf("failed to create backup: %v", err))
	}

	// 序列化并写回配置文件
	output, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	common.SysLog(fmt.Sprintf("Removed direct rule for %s from %s", domain, configPath))
	return nil
}

// extractDomain 从 URL 中提取域名
func extractDomain(baseURL string) string {
	// 移除协议前缀
	url := strings.TrimPrefix(baseURL, "http://")
	url = strings.TrimPrefix(url, "https://")

	// 移除路径和端口
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}
	if idx := strings.Index(url, ":"); idx != -1 {
		url = url[:idx]
	}

	return strings.TrimSpace(url)
}
