package main

import (
	"errors"
	"flag"
	"fmt"
	"go/build"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/d--j/go-milter/integration"
)

type Config struct {
	ReceiverPort uint16
	MilterPort   uint16
	ScratchDir   string
	MTAs         []string
	TestDirs     []*TestDir
	Tests        []*TestCase
	Filter       *regexp.Regexp
}

func (c *Config) Cleanup() {
	if c.ScratchDir != "" {
		_ = os.RemoveAll(c.ScratchDir)
	}
}

func ParseConfig() *Config {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("could not get path to runner.go")
	}
	mtaPath := ""
	flag.StringVar(&mtaPath, "mta", path.Join(path.Dir(path.Dir(filename)), "mta"), "`path` to MTA definitions")
	filter := ""
	flag.StringVar(&filter, "filter", "", "regexp `pattern` to filter testcases")
	mtaFilter := ""
	flag.StringVar(&mtaFilter, "mtaFilter", "", "regexp `pattern` to filter MTAs")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "  test-dir...\n    \tone ore more directories containing test filters and testcases\n")
	}
	flag.Parse()
	if filter == "" {
		filter = ".*"
	}
	filterRe, err := regexp.Compile(filter)
	if err != nil {
		LevelOneLogger.Fatal(err)
	}
	if mtaFilter == "" {
		mtaFilter = ".*"
	}
	mtaFilterRe, err := regexp.Compile(mtaFilter)
	if err != nil {
		LevelOneLogger.Fatal(err)
	}
	config := Config{
		Filter:     filterRe,
		ScratchDir: "",
	}
	tmpDir, err := os.MkdirTemp("", "scratch-*")
	if err != nil {
		LevelOneLogger.Fatal(err)
	}
	err = os.Chmod(tmpDir, 0755)
	if err != nil {
		LevelOneLogger.Fatal(err)
	}
	config.ScratchDir = tmpDir
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}
	testDirs, err := expandTestDirs(flag.Args())
	if err != nil {
		LevelOneLogger.Fatalf("error getting tests: %s", err)
	}
	mtas, err := filepath.Glob(path.Join(mtaPath, "*/mta.sh"))
	if err != nil {
		LevelOneLogger.Fatalf("error getting MTAs: %s", err)
	}
	if mtas == nil {
		LevelOneLogger.Fatalf("did not find any MTAs")
	}
	var filteredMtas []string
	for _, m := range mtas {
		if mtaFilterRe.MatchString(m) {
			filteredMtas = append(filteredMtas, m)
		}
	}
	var dirs []*TestDir
	var tests []*TestCase
	for _, p := range filteredMtas {
		mta, err := NewMTA(p, uint16(getAvailablePort()), &config)
		if err != nil {
			LevelOneLogger.Printf("SKIP %s: %s", p, err)
			continue
		}
		if mta == nil {
			LevelOneLogger.Printf("SKIP %s: empty tag list", p)
			continue
		}

		for i, testDir := range testDirs {
			dir := TestDir{
				Index:  i,
				Path:   testDir,
				Config: &config,
				MTA:    mta,
			}
			err = filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
				if !d.IsDir() {
					if filepath.Ext(path) == ".testcase" && filterRe.MatchString(path) {
						testCase, err := integration.ParseTestCase(path)
						if err != nil {
							return fmt.Errorf("parsing %s: %w", path, err)
						}
						test := &TestCase{
							Index:    len(tests),
							Filename: filepath.Base(path),
							TestCase: testCase,
							parent:   &dir,
						}
						dir.Tests = append(dir.Tests, test)
						tests = append(tests, test)
					}
				} else if path != testDir {
					return filepath.SkipDir
				}
				return nil
			})
			if err != nil {
				LevelOneLogger.Fatal(err)
			}
			if len(dir.Tests) > 0 {
				dirs = append(dirs, &dir)
			}
		}
	}
	if len(tests) == 0 {
		LevelOneLogger.Fatal("did not find any tests")
	}

	config.ReceiverPort = getAvailablePort()
	config.MilterPort = getAvailablePort()
	config.MTAs = mtas
	config.TestDirs = dirs
	config.Tests = tests

	if err := GenCert("localhost.local", config.ScratchDir); err != nil {
		LevelOneLogger.Fatal(err)
	}

	LevelOneLogger.Printf("OK %d test cases", len(tests))

	return &config
}

var tagsSplit = regexp.MustCompile("[\n\r]")

func removeEmptyOrDuplicates(str []string) []string {
	if len(str) == 0 {
		return []string{}
	}
	found := make(map[string]bool, len(str))
	indexesToKeep := make([]int, 0, len(str))
	found[""] = true
	for i, v := range str {
		v = strings.TrimSpace(v)
		if !found[v] {
			indexesToKeep = append(indexesToKeep, i)
			found[v] = true
		}
	}
	noDuplicates := make([]string, len(indexesToKeep))
	for i, index := range indexesToKeep {
		noDuplicates[i] = strings.TrimSpace(str[index])
	}
	return noDuplicates
}

func expandTestDirs(in []string) (dirs []string, err error) {
	ctxt := build.Default // copy
	ctxt.UseAllFiles = true
	for len(in) > 0 {
		candidate, err := filepath.Abs(in[0])
		in = in[1:]
		if err != nil {
			return nil, err
		}
		if stat, err := os.Stat(candidate); err != nil || !stat.IsDir() {
			return nil, fmt.Errorf("path %s is not a directory", candidate)
		}
		pkg, err := ctxt.ImportDir(candidate, 0)
		if err != nil {
			var noGoError *build.NoGoError
			if errors.As(err, &noGoError) {
				err = filepath.WalkDir(candidate, func(path string, d fs.DirEntry, err error) error {
					if err == nil && candidate != path && d.IsDir() {
						in = append(in, path)
					}
					if d.IsDir() && candidate != path {
						return filepath.SkipDir
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			}
		} else {
			if !pkg.IsCommand() {
				return nil, fmt.Errorf("path %s contains package %s not main", candidate, pkg.Name)
			}
			dirs = append(dirs, candidate)
		}

	}
	if len(dirs) == 0 {
		return nil, errors.New("could not find any tests")
	}
	return
}

func getAvailablePort() uint16 {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return uint16(port)
}
