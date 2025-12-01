package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/roblaszczak/go-cleanarch/cleanarch"
	"gopkg.in/yaml.v3"
)

type config struct {
	Version           int      `yaml:"version"`
	Root              string   `yaml:"root"`
	IgnoreTests       bool     `yaml:"ignore_tests"`
	IgnorePackages    []string `yaml:"ignore_packages"`
	SharedModules     []string `yaml:"shared_modules"`
	AllowedViolations []string `yaml:"allow_violations"`
	Aliases           struct {
		Domain         []string `yaml:"domain"`
		Application    []string `yaml:"application"`
		Interfaces     []string `yaml:"interfaces"`
		Infrastructure []string `yaml:"infrastructure"`
	} `yaml:"aliases"`
}

func main() {
	var (
		configPath = flag.String("config", ".gocleanarch.yml", "配置文件路径")
		debug      = flag.Bool("debug", false, "开启 go-cleanarch 调试日志")
	)

	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v\n", err)
	}

	root, err := resolveRoot(cfg.Root)
	if err != nil {
		log.Fatalf("解析 root 失败: %v\n", err)
	}

	aliases := map[string]cleanarch.Layer{}
	applyAliases(aliases, cfg.Aliases.Domain, defaultDomainAliases, cleanarch.LayerDomain)
	applyAliases(aliases, cfg.Aliases.Application, defaultApplicationAliases, cleanarch.LayerApplication)
	applyAliases(aliases, cfg.Aliases.Interfaces, defaultInterfacesAliases, cleanarch.LayerInterfaces)
	applyAliases(aliases, cfg.Aliases.Infrastructure, defaultInfrastructureAliases, cleanarch.LayerInfrastructure)

	if *debug {
		cleanarch.Log.SetOutput(os.Stderr)
	}

	validator := cleanarch.NewValidator(aliases)

	ok, errs, err := validator.Validate(root, cfg.IgnoreTests, cfg.IgnorePackages)
	if err != nil {
		log.Fatalf("执行 go-cleanarch 失败: %v\n", err)
	}

	filtered := filterValidationErrors(errs, cfg)
	if !ok && len(filtered) > 0 {
		for _, validationErr := range filtered {
			log.Println(validationErr.Error())
		}
		log.Println("go-cleanarch 检查未通过。")
		os.Exit(1)
	}

	log.Println("go-cleanarch 检查通过。")
}

func loadConfig(path string) (*config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.Root == "" {
		cfg.Root = "."
	}

	return cfg, nil
}

func resolveRoot(root string) (string, error) {
	if root == "" {
		return "", errors.New("root 不能为空")
	}
	return filepath.Abs(root)
}

var (
	defaultDomainAliases         = []string{"domain", "entities"}
	defaultApplicationAliases    = []string{"app", "application", "usecases", "usecase", "use_cases"}
	defaultInterfacesAliases     = []string{"interfaces", "interface", "adapters", "adapter"}
	defaultInfrastructureAliases = []string{"infrastructure", "infra"}
)

func applyAliases(dst map[string]cleanarch.Layer, custom []string, defaults []string, layer cleanarch.Layer) {
	candidates := defaults
	if len(custom) > 0 {
		candidates = custom
	}

	for _, alias := range candidates {
		if alias == "" {
			continue
		}
		dst[alias] = layer
	}
}

var crossModulePattern = regexp.MustCompile(`between ([\w-]+) and ([\w-]+) modules`)

func filterValidationErrors(errs []cleanarch.ValidationError, cfg *config) []cleanarch.ValidationError {
	if len(errs) == 0 {
		return nil
	}

	shared := make(map[string]struct{}, len(cfg.SharedModules))
	for _, module := range cfg.SharedModules {
		module = strings.TrimSpace(module)
		if module == "" {
			continue
		}
		shared[module] = struct{}{}
	}

	allowedPatterns := make([]string, 0, len(cfg.AllowedViolations))
	for _, pattern := range cfg.AllowedViolations {
		if pattern != "" {
			allowedPatterns = append(allowedPatterns, pattern)
		}
	}

	filtered := make([]cleanarch.ValidationError, 0, len(errs))
	for _, validationErr := range errs {
		msg := validationErr.Error()
		if skipCrossModule(msg, shared) {
			continue
		}
		if containsAllowedPattern(msg, allowedPatterns) {
			continue
		}
		filtered = append(filtered, validationErr)
	}

	return filtered
}

func skipCrossModule(msg string, shared map[string]struct{}) bool {
	if len(shared) == 0 {
		return false
	}

	matches := crossModulePattern.FindStringSubmatch(msg)
	if len(matches) != 3 {
		return false
	}

	if _, ok := shared[matches[1]]; ok {
		return true
	}
	if _, ok := shared[matches[2]]; ok {
		return true
	}

	return false
}

func containsAllowedPattern(msg string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}
