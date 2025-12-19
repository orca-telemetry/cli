package stub

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	pb "github.com/orc-analytics/core/protobufs/go"
)

const PYTHON_STUB_FILE = "stub_templates/processor.py.tmpl"

//go:embed stub_templates/*.tmpl
var templateFS embed.FS

var pythonTemplate *template.Template

type ReturnType string

const (
	structReturnType ReturnType = "StructResult"
	valueReturnType  ReturnType = "ValueResult"
	noneReturnType   ReturnType = "NoneResult"
	arrayReturnType  ReturnType = "ArrayResult"
)

func init() {
	baseName := filepath.Base(PYTHON_STUB_FILE)
	pythonTemplate = template.Must(template.New(baseName).Funcs(
		template.FuncMap{
			"ToSnakeCase": toSnakeCase,
		}).ParseFS(templateFS, PYTHON_STUB_FILE))
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		if r >= 'A' && r <= 'Z' {
			result = append(result, r+32)
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// data structures matching the template expectations
type Metadata struct {
	VarName     string
	KeyName     string
	Description string
}

type Window struct {
	VarName          string
	Name             string
	Version          string
	Description      string
	MetadataVarNames []string
}

type Algorithm struct {
	Name          string
	Version       string
	WindowVarName string
	ReturnType    ReturnType
}

type ProcessorData struct {
	Name       string
	Metadata   []Metadata
	Windows    []Window
	Algorithms []Algorithm
}

type AllProcessors struct {
	Processors []ProcessorData
}

func mapInternalStateToTmpl(internalState *pb.InternalState) (error, *AllProcessors) {
	processorDatas := make([]ProcessorData, len(internalState.GetProcessors()))

	for ii, proc := range internalState.GetProcessors() {
		supportedWindowTypes := make(map[string]*Window)
		supportedWindowMetadataFields := make(map[string]*Metadata)
		supportedAlgorithms := make([]Algorithm, len(proc.GetSupportedAlgorithms()))

		for jj, algo := range proc.GetSupportedAlgorithms() {
			windowKey := fmt.Sprintf("%v_%v", algo.GetWindowType().GetName(), algo.GetWindowType().GetVersion())

			// Pack all the window metadata fields
			metadataVarNamesForWindow := make([]string, len(algo.GetWindowType().GetMetadataFields()))
			for kk, metadata := range algo.GetWindowType().GetMetadataFields() {
				metadataVarName := fmt.Sprintf("%v_stub", metadata.GetName())

				if _, ok := supportedWindowMetadataFields[metadata.GetName()]; !ok {
					supportedWindowMetadataFields[metadata.GetName()] = &Metadata{
						VarName:     metadataVarName,
						KeyName:     metadata.GetName(),
						Description: metadata.GetDescription(),
					}
				}
				metadataVarNamesForWindow[kk] = metadataVarName
			}

			// Pack all the window types
			if _, ok := supportedWindowTypes[windowKey]; !ok {
				supportedWindowTypes[windowKey] = &Window{
					VarName:          fmt.Sprintf("%v_stub", windowKey),
					Name:             algo.GetWindowType().GetName(),
					Version:          algo.GetWindowType().GetVersion(),
					Description:      algo.GetWindowType().GetDescription(),
					MetadataVarNames: metadataVarNamesForWindow,
				}
			}

			var algoReturnType ReturnType
			switch algo.GetResultType() {
			case pb.ResultType_ARRAY:
				algoReturnType = arrayReturnType
			case pb.ResultType_STRUCT:
				algoReturnType = structReturnType
			case pb.ResultType_VALUE:
				algoReturnType = valueReturnType
			case pb.ResultType_NONE:
				algoReturnType = noneReturnType
			case pb.ResultType_NOT_SPECIFIED:
				return fmt.Errorf(
					"result type not specified for algorithm %v_%v on processor %v_%v",
					algo.GetName(),
					algo.GetVersion(),
					proc.GetName(),
					proc.GetRuntime(),
				), nil
			}

			supportedAlgorithms[jj] = Algorithm{
				Name:          algo.GetName(),
				Version:       algo.GetVersion(),
				ReturnType:    algoReturnType,
				WindowVarName: fmt.Sprintf("%v_stub", algo.GetWindowType().GetName()),
			}
		}

		allWindowTypes := make([]Window, 0, len(supportedWindowTypes))
		for _, windowTypePtr := range supportedWindowTypes {
			allWindowTypes = append(allWindowTypes, *windowTypePtr)
		}

		allMetadataFields := make([]Metadata, 0, len(supportedWindowMetadataFields))
		for _, metadataPtr := range supportedWindowMetadataFields {
			allMetadataFields = append(allMetadataFields, *metadataPtr)
		}

		processorDatas[ii] = ProcessorData{
			Name:       proc.GetName(),
			Metadata:   allMetadataFields,
			Windows:    allWindowTypes,
			Algorithms: supportedAlgorithms,
		}
	}

	return nil, &AllProcessors{Processors: processorDatas}
}

func GeneratePythonStub(internalState *pb.InternalState, outDir string) error {

	err, processorTmpl := mapInternalStateToTmpl(internalState)
	if err != nil {
		return fmt.Errorf("could not parse internal state: %w", err)
	}

	outFile, err := os.Create("orca_stub.py")

	if err != nil && !os.IsExist(err) {
		return err
	}

	defer outFile.Close()

	err = os.Mkdir(outDir, 0750)

	if err != nil && !os.IsExist(err) {
		return (err)
	}
	if err := pythonTemplate.Execute(outFile, processorTmpl); err != nil {
		panic(err)
	}
	println("Generated system_state.py successfully!")
	return nil
}
