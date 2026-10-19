package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func parseEnvPairs(pairs []string) (map[string]string, error) {
	env := make(map[string]string)
	for _, pair := range pairs {
		if pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid env var: %s", pair)
		}
		env[key] = value
	}
	return env, nil
}

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %s", pattern)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

// executionDefaults mirrors the donor options.execution config surface.
//
// TODO(port-wave4): the chassis config has no options.execution surface
// yet; callers pass nil and flags remain the only source.
type executionDefaults struct {
	Shell           string
	TimeoutSeconds  int
	MaxBufferBytes  int
	AllowedPrefixes []string
	AllowedPatterns []string
	DeniedPrefixes  []string
	DeniedPatterns  []string
}

func resolveExecutionConfig(cmd *cobra.Command, execCfg *executionDefaults) (executionConfigInputs, error) {
	shell, _ := cmd.Flags().GetString("shell")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	maxBuffer, _ := cmd.Flags().GetInt("max-buffer")
	allowPrefixes, _ := cmd.Flags().GetStringArray("allow-prefix")
	allowPatterns, _ := cmd.Flags().GetStringArray("allow-regex")
	denyPrefixes, _ := cmd.Flags().GetStringArray("deny-prefix")
	denyPatterns, _ := cmd.Flags().GetStringArray("deny-regex")
	envPairs, _ := cmd.Flags().GetStringArray("env")

	env, err := parseEnvPairs(envPairs)
	if err != nil {
		return executionConfigInputs{}, err
	}

	prefixes := []string{}
	if execCfg != nil && len(execCfg.AllowedPrefixes) > 0 {
		prefixes = append(prefixes, execCfg.AllowedPrefixes...)
	}
	if cmd.Flags().Changed("allow-prefix") {
		prefixes = append(prefixes, allowPrefixes...)
	}

	patternStrings := []string{}
	if execCfg != nil && len(execCfg.AllowedPatterns) > 0 {
		patternStrings = append(patternStrings, execCfg.AllowedPatterns...)
	}
	if cmd.Flags().Changed("allow-regex") {
		patternStrings = append(patternStrings, allowPatterns...)
	}

	compiledPatterns, err := compilePatterns(patternStrings)
	if err != nil {
		return executionConfigInputs{}, err
	}

	denyPrefixList := []string{}
	if execCfg != nil && len(execCfg.DeniedPrefixes) > 0 {
		denyPrefixList = append(denyPrefixList, execCfg.DeniedPrefixes...)
	}
	if cmd.Flags().Changed("deny-prefix") {
		denyPrefixList = append(denyPrefixList, denyPrefixes...)
	}

	denyPatternStrings := []string{}
	if execCfg != nil && len(execCfg.DeniedPatterns) > 0 {
		denyPatternStrings = append(denyPatternStrings, execCfg.DeniedPatterns...)
	}
	if cmd.Flags().Changed("deny-regex") {
		denyPatternStrings = append(denyPatternStrings, denyPatterns...)
	}

	compiledDenied, err := compilePatterns(denyPatternStrings)
	if err != nil {
		return executionConfigInputs{}, err
	}

	resolvedShell := shell
	if resolvedShell == "" && execCfg != nil {
		resolvedShell = execCfg.Shell
	}

	resolvedTimeout := timeout
	if resolvedTimeout == 0 && execCfg != nil && execCfg.TimeoutSeconds > 0 {
		resolvedTimeout = time.Duration(execCfg.TimeoutSeconds) * time.Second
	}

	resolvedMaxBuffer := maxBuffer
	if resolvedMaxBuffer == 0 && execCfg != nil && execCfg.MaxBufferBytes > 0 {
		resolvedMaxBuffer = execCfg.MaxBufferBytes
	}

	return executionConfigInputs{
		Shell:          resolvedShell,
		Timeout:        resolvedTimeout,
		MaxBufferBytes: resolvedMaxBuffer,
		AllowedPrefix:  prefixes,
		AllowedRegex:   compiledPatterns,
		DeniedPrefix:   denyPrefixList,
		DeniedRegex:    compiledDenied,
		Env:            env,
	}, nil
}

type executionConfigInputs struct {
	Shell          string
	Timeout        time.Duration
	MaxBufferBytes int
	AllowedPrefix  []string
	AllowedRegex   []*regexp.Regexp
	DeniedPrefix   []string
	DeniedRegex    []*regexp.Regexp
	Env            map[string]string
}
