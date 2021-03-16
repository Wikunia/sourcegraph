package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	"github.com/google/go-cmp/cmp"
	"github.com/inconshreveable/log15"
	"github.com/pelletier/go-toml"
	"github.com/sourcegraph/sourcegraph/enterprise/lib/codeintel/lsif/conversion"
	"github.com/sourcegraph/sourcegraph/enterprise/lib/codeintel/semantic"
	"github.com/sourcegraph/sourcegraph/internal/logging"
	"github.com/sourcegraph/sourcegraph/internal/trace"
)

// TODO:
//   We need to check for lsif-clang, lsif-validate at start time.

type ProjectResult struct {
	name    string
	success bool
	usage   UsageStats
	output  string
}

type IndexerResult struct {
	usage  UsageStats
	output string
}

type UsageStats struct {
	// Memory usage in kilobytes by child process.
	memory int64
}

// const map[string][]string {
// 	"lsif-clang": []string{"lsif-clang", "compile_commands.json"},
// }
var indexer_commands = map[string]func(string) (string, []string){
	"lsif-clang": func(directory string) (string, []string) {
		return "lsif-clang", []string{"compile_commands.json"}
	},
}

var directory string
var indexer string
var monitor bool

func main() {
	logging.Init()
	trace.Init(false)

	log15.Root().SetHandler(log15.StdoutHandler)

	flag.StringVar(&directory, "dir", ".", "The directory to run the test harness over")
	flag.StringVar(&indexer, "indexer", "", "The name of the indexer that you want to test")
	flag.BoolVar(&monitor, "monitor", true, "Whether to monitor and log stats")

	flag.Parse()

	if indexer == "" {
		log.Fatalf("Indexer is required. Pass with --indexer")
	}

	log15.Info("Starting Execution: ", "base directory", directory, "indexer", indexer)

	testContext := context.Background()

	err := testDirectory(testContext, indexer, directory)
	if err != nil {
		log.Fatalf("Failed with: %s", err)
	}
}

func testDirectory(ctx context.Context, indexer string, directory string) error {
	files, err := ioutil.ReadDir(directory)
	if err != nil {
		return err
	}

	results := make(chan *ProjectResult)

	for _, f := range files {
		go func(name string) {
			projResult, err := testProject(ctx, indexer, directory+"/"+name, name)

			if err != nil {
				results <- nil
			} else {
				results <- &projResult
			}
		}(f.Name())

	}

	for range files {
		projResult := <-results

		if projResult == nil {
			log15.Info("This is weird and it failed...")
			continue
		}

		log15.Info("Project result:", "name", projResult.name, "success", projResult.success)
		if !projResult.success {
			log.Fatalf("Project '%s' failed test", "project")
		}
	}

	return nil
}

func testProject(ctx context.Context, indexer, project, name string) (ProjectResult, error) {
	output, err := setupProject(project)
	if err != nil {
		return ProjectResult{name: name, success: false, output: string(output)}, err
	} else {
		log15.Debug("... Generated compile_commands.json")
	}

	result, err := runIndexer(ctx, indexer, project, name)
	if err != nil {
		return ProjectResult{
			name:    name,
			success: false,
			output:  string(result.output),
		}, err
	}

	log15.Debug("... \t Resource Usage:", "usage", result.usage)

	output, err = validateDump(project)
	if err != nil {
		fmt.Println("Not valid")
		return ProjectResult{
			name:    name,
			success: false,
			usage:   result.usage,
			output:  string(output),
		}, err
	} else {
		log15.Debug("... Validated dump.lsif")
	}

	bundle, err := readBundle(1, project)
	if err != nil {
		return ProjectResult{
			name:    name,
			success: false,
			usage:   result.usage,
			output:  string(output),
		}, err
	}

	validateTestCases(project, bundle)

	return ProjectResult{
		name:    name,
		success: true,
		usage:   result.usage,
		output:  string(output),
	}, nil
}

func setupProject(directory string) ([]byte, error) {
	cmd := exec.Command("./setup_indexer.sh")
	cmd.Dir = directory

	return cmd.CombinedOutput()
}

func runIndexer(ctx context.Context, indexer, directory, name string) (ProjectResult, error) {
	// TODO: We should add how long it takes to generate this.
	commandGetter, ok := indexer_commands[indexer]
	if !ok {
		panic("Invalid indexer")
	}

	command, args := commandGetter(directory)

	log15.Debug("... Generating dump.lsif")
	cmd := exec.Command(command, args...)
	cmd.Dir = directory

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ProjectResult{}, err
	}

	sysUsage := cmd.ProcessState.SysUsage()
	mem, _ := MaxMemoryInKB(sysUsage)
	// fmt.Println("Memory Usage:", mem, "kB")
	// fmt.Println("User CPU", sysUsage.Utime)

	return ProjectResult{
		name:    name,
		success: false,
		usage:   UsageStats{memory: mem},
		output:  string(output),
	}, err
}

func validateDump(directory string) ([]byte, error) {
	// TODO: Eventually this should use the package, rather than the installed module
	//       but for now this will have to do.
	cmd := exec.Command("lsif-validate", "dump.lsif")
	cmd.Dir = directory

	return cmd.CombinedOutput()
}

func validateTestCases(directory string, bundle *conversion.GroupedBundleDataMaps) {
	doc, err := ioutil.ReadFile(directory + "/test.toml")
	if err != nil {
		log15.Warn("No file exists here")
		return
	}

	testCase := LsifTest{}
	toml.Unmarshal(doc, &testCase)

	for _, definitionRequest := range testCase.Definitions {
		path := definitionRequest.Request.TextDocument
		line := definitionRequest.Request.Position.Line
		character := definitionRequest.Request.Position.Character

		results, err := conversion.Query(bundle, path, line, character)

		if err != nil {
			log.Fatalf("Failed query: %s", err)
		}

		if len(results) > 1 {
			log.Fatalf("Had too many results: %v", results)
		} else if len(results) == 0 {
			log.Fatalf("Found no results: %v", results)
		}

		definitions := results[0].Definitions

		if len(definitions) > 1 {
			log.Fatalf("Had too many definitions: %v", definitions)
		} else if len(definitions) == 0 {
			log.Fatalf("Found no definitions: %v", definitions)
		}

		response := transformLocationToResponse(definitions[0])
		if diff := cmp.Diff(response, definitionRequest.Response); diff != "" {
			log.Fatalf("Bad diffs: %s", diff)
		}
	}

	log15.Info("Passed tests", "project", directory)
}

func transformLocationToResponse(location semantic.LocationData) DefinitionResponse {
	return DefinitionResponse{
		TextDocument: location.URI,
		Range: Range{
			Start: Position{
				Line:      location.StartLine,
				Character: location.StartCharacter,
			},
			End: Position{
				Line:      location.EndLine,
				Character: location.EndCharacter,
			},
		},
	}

}

func getWriter(ctx context.Context) *os.File {
	return ctx.Value("output").(*os.File)
}
