package appdetect

import (
	"context"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/azure/azure-dev/cli/azd/internal/tracing"
	"github.com/azure/azure-dev/cli/azd/internal/tracing/fields"
	"github.com/azure/azure-dev/cli/azd/pkg/tools/maven"
)

type javaDetector struct {
	mvnCli     *maven.Cli
	rootPoms   []pom
	modulePoms map[string]pom
}

// JavaProjectOptionParentPomDir The parent module path of the maven multi-module project
const JavaProjectOptionParentPomDir = "parentPath"

func (jd *javaDetector) Language() Language {
	return Java
}

func (jd *javaDetector) DetectProject(ctx context.Context, path string, entries []fs.DirEntry) (*Project, error) {
	for _, entry := range entries {
		if strings.ToLower(entry.Name()) == "pom.xml" { // todo: support file names like backend-pom.xml
			tracing.SetUsageAttributes(fields.AppInitJavaDetect.String("start"))
			pomPath := filepath.Join(path, entry.Name())
			mavenProject, err := createMavenProject(ctx, jd.mvnCli, pomPath)
			if err != nil {
				log.Printf("Please edit azure.yaml manually to satisfy your requirement. azd can not help you "+
					"to that by detect your java project because error happened when reading pom.xml: %s. ", err)
				return nil, nil
			}

			if len(mavenProject.pom.Modules) > 0 {
				// This is a multi-module project, we will capture the analysis, but return nil to continue recursing
				if _, ok := jd.modulePoms[mavenProject.pom.pomFilePath]; !ok {
					jd.rootPoms = append(jd.rootPoms, mavenProject.pom)
				}
				for _, module := range mavenProject.pom.Modules {
					var modulePath string
					if strings.HasSuffix(module, ".xml") {
						modulePath = filepath.Join(path, module)
					} else {
						modulePath = filepath.Join(path, module, "pom.xml")
					}
					jd.modulePoms[modulePath] = mavenProject.pom
					for {
						if result, ok := jd.modulePoms[jd.modulePoms[modulePath].pomFilePath]; ok {
							jd.modulePoms[modulePath] = result
						} else {
							break
						}
					}
				}
				return nil, nil
			}

			if !isSpringBootRunnableProject(mavenProject) {
				return nil, nil
			}

			var parentPom *pom
			for _, parentPomItem := range jd.rootPoms {
				// we can say that the project is in the root project if
				// 1) the project path is under the root project
				// 2) the project is under the modules of root project
				underRootPath := strings.HasPrefix(pomPath, filepath.Dir(parentPomItem.pomFilePath)+string(filepath.Separator))
				rootPomItem, exist := jd.modulePoms[mavenProject.pom.pomFilePath]
				if underRootPath && exist && rootPomItem.pomFilePath == parentPomItem.pomFilePath {
					parentPom = &parentPomItem
					break
				}
			}

			project := Project{
				Language:      Java,
				Path:          path,
				DetectionRule: "Inferred by presence of: pom.xml",
			}
			detectAzureDependenciesByAnalyzingSpringBootProject(mavenProject, &project)
			if parentPom != nil {
				project.Options = map[string]interface{}{
					JavaProjectOptionParentPomDir: filepath.Dir(parentPom.pomFilePath),
				}
			}

			tracing.SetUsageAttributes(fields.AppInitJavaDetect.String("finish"))
			return &project, nil
		}
	}
	return nil, nil
}

// inParentModules recursively descends the modules of parentPom to determines if the currentPom is submodule
func inParentModules(currentPom pom, parentPom pom, parentPoms []pom) bool {
	if inModule(currentPom, parentPom) {
		return true
	}

	for _, module := range parentPom.Modules {
		for _, pomItem := range parentPoms {
			if module == filepath.Base(filepath.Dir(pomItem.pomFilePath)) {
				return inParentModules(currentPom, pomItem, parentPoms)
			}
		}
	}
	return false
}

func inModule(currentPom pom, parentPom pom) bool {
	for _, module := range parentPom.Modules {
		if module == filepath.Base(filepath.Dir(currentPom.pomFilePath)) ||
			module == filepath.Base(currentPom.pomFilePath) {
			return true
		}
	}
	return false
}
