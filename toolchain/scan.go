package golang

import (
	"bytes"
	"encoding/json"
	"path/filepath"

	"strings"

	"sourcegraph.com/sourcegraph/srcgraph/config"
	"sourcegraph.com/sourcegraph/srcgraph/container"
	"sourcegraph.com/sourcegraph/srcgraph/scan"
	"sourcegraph.com/sourcegraph/srcgraph/task2"
	"sourcegraph.com/sourcegraph/srcgraph/unit"
)

func init() {
	scan.Register("go", scan.DockerScanner{defaultGoVersion})
}

func (v *goVersion) BuildScanner(dir string, c *config.Repository, x *task2.Context) (*container.Command, error) {
	goConfig := v.goConfig(c)

	dockerfile, err := v.baseDockerfile()
	if err != nil {
		return nil, err
	}

	containerDir := filepath.Join(containerGOPATH, "src", goConfig.BaseImportPath)
	cont := container.Container{
		Dockerfile: dockerfile,
		RunOptions: []string{"-v", dir + ":" + containerDir},
		Cmd:        []string{"go", "list", "-e", "-json", goConfig.BaseImportPath + "/..."},
		Stderr:     x.Stderr,
		Stdout:     x.Stdout,
	}
	cmd := container.Command{
		Container: cont,
		Transform: func(orig []byte) ([]byte, error) {
			if len(orig) == 0 {
				return nil, nil
			}

			pkgs := bytes.SplitAfter(bytes.TrimSpace(orig), []byte("\n}\n"))
			units := make([]unit.SourceUnit, len(pkgs))
			for i, pkgJSON := range pkgs {
				var pkg map[string]interface{}
				err := json.Unmarshal(pkgJSON, &pkg)
				if err != nil {
					return nil, err
				}

				importPath := pkg["ImportPath"].(string)
				dir, err := filepath.Rel(goConfig.BaseImportPath, importPath)
				if err != nil {
					return nil, err
				}

				// collect all filenames
				var files []string
				for k, v := range pkg {
					if strings.HasSuffix(k, "Files") {
						if list, ok := v.([]interface{}); ok {
							for _, file := range list {
								files = append(files, file.(string))
							}
						}
					}
				}

				units[i] = &Package{
					Dir:        dir,
					ImportPath: importPath,
					Files:      files,
				}
			}
			return json.Marshal(units)
		},
	}
	return &cmd, nil
}

func (v *goVersion) UnmarshalSourceUnits(data []byte) ([]unit.SourceUnit, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var goPackages []*Package
	err := json.Unmarshal(data, &goPackages)
	if err != nil {
		return nil, err
	}

	units := make([]unit.SourceUnit, len(goPackages))
	for i, p := range goPackages {
		units[i] = p
	}

	return units, nil
}