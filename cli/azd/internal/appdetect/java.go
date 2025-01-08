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
	parentPoms []pom
}

// JavaProjectOptionCurrentPomDir The project path of the maven single-module project
const JavaProjectOptionCurrentPomDir = "path"

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
				// This is a multi-module project, we will capture the analysis, but return nil
				// to continue recursing
				jd.parentPoms = append(jd.parentPoms, mavenProject.pom)
				return nil, nil
			}

			if !isSpringBootRunnableProject(mavenProject) {
				return nil, nil
			}

			var parentPom *pom
			for _, parentPomItem := range jd.parentPoms {
				// we can say that the project is in the root project if
				// 1) the project path is under the root project
				// 2) the project is under the modules of root project
				inRoot := strings.HasPrefix(pomPath, filepath.Dir(parentPomItem.pomFilePath)+string(filepath.Separator))
				if inRoot && inParentModules(mavenProject.pom, parentPomItem, jd.parentPoms) {
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
			} else {
				project.Options = map[string]interface{}{
					JavaProjectOptionCurrentPomDir: path,
				}
			}

			tracing.SetUsageAttributes(fields.AppInitJavaDetect.String("finish"))
			return &project, nil
		}
	}
	return nil, nil
}

func detectMavenWrapper(path string, executable string) string {
	wrapperPath := filepath.Join(path, executable)
	if fileExists(wrapperPath) {
		return wrapperPath
	}
	return ""
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
		if module == filepath.Base(filepath.Dir(currentPom.pomFilePath)) {
			return true
		}
	}
	return false
}
