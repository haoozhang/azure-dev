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
	mvnCli            *maven.Cli
	parentPoms        []pom
	mavenWrapperPaths []mavenWrapper
}

type mavenWrapper struct {
	posixPath string
	winPath   string
}

// JavaProjectOptionCurrentPomDir The project path of the maven single-module project
const JavaProjectOptionCurrentPomDir = "path"

// JavaProjectOptionParentPomDir The parent module path of the maven multi-module project
const JavaProjectOptionParentPomDir = "parentPath"

// JavaProjectOptionPosixMavenWrapperPath The path to the maven wrapper script for POSIX systems
const JavaProjectOptionPosixMavenWrapperPath = "posixMavenWrapperPath"

// JavaProjectOptionWinMavenWrapperPath The path to the maven wrapper script for Windows systems
const JavaProjectOptionWinMavenWrapperPath = "winMavenWrapperPath"

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
				jd.mavenWrapperPaths = append(jd.mavenWrapperPaths, mavenWrapper{
					posixPath: detectMavenWrapper(path, "mvnw"),
					winPath:   detectMavenWrapper(path, "mvnw.cmd"),
				})
				return nil, nil
			}

			if !isSpringBootRunnableProject(mavenProject.pom) {
				return nil, nil
			}

			var parentPom *pom
			var currentWrapper mavenWrapper
			for i, parentPomItem := range jd.parentPoms {
				// we can say that the project is in the root project if
				// 1) the path is under the project
				// 2) the artifact is under the parent modules
				if inRoot := strings.HasPrefix(pomPath, filepath.Dir(parentPomItem.pomFilePath)); inRoot {
					if inParentModules(mavenProject.pom, parentPomItem.ArtifactId, jd.parentPoms) {
						parentPom = &parentPomItem
						currentWrapper = jd.mavenWrapperPaths[i]
						break
					}
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
					JavaProjectOptionParentPomDir:          filepath.Dir(parentPom.pomFilePath),
					JavaProjectOptionPosixMavenWrapperPath: currentWrapper.posixPath,
					JavaProjectOptionWinMavenWrapperPath:   currentWrapper.winPath,
				}
			} else {
				project.Options = map[string]interface{}{
					JavaProjectOptionCurrentPomDir:         path,
					JavaProjectOptionPosixMavenWrapperPath: detectMavenWrapper(path, "mvnw"),
					JavaProjectOptionWinMavenWrapperPath:   detectMavenWrapper(path, "mvnw.cmd"),
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

// isSpringBootRunnableProject checks if the pom indicates a runnable Spring Boot project
func isSpringBootRunnableProject(pom pom) bool {
	targetGroupId := "org.springframework.boot"
	targetArtifactId := "spring-boot-maven-plugin"
	for _, plugin := range pom.Build.Plugins {
		if plugin.GroupId == targetGroupId && plugin.ArtifactId == targetArtifactId {
			return true
		}
	}
	return false
}

// inParentModules recursively determines if the pom is the submodule of the given parentPom
func inParentModules(currentPom pom, parentPomArtifactId string, jdParentPoms []pom) bool {
	springBootStarterParentArtifactId := "spring-boot-starter-parent"
	if currentPom.Parent.ArtifactId == springBootStarterParentArtifactId {
		return false
	}

	if currentPom.Parent.ArtifactId == parentPomArtifactId {
		return true
	}

	for _, pom := range jdParentPoms {
		if pom.ArtifactId == currentPom.Parent.ArtifactId {
			return inParentModules(pom, parentPomArtifactId, jdParentPoms)
		}
	}
	return false
}
