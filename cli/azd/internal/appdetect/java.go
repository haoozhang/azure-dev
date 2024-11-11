package appdetect

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/azure/azure-dev/cli/azd/internal/tracing"
	"github.com/azure/azure-dev/cli/azd/internal/tracing/fields"
	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/braydonk/yaml"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type javaDetector struct {
	rootProjects []mavenProject
}

func (jd *javaDetector) Language() Language {
	return Java
}

func (jd *javaDetector) DetectProject(ctx context.Context, path string, entries []fs.DirEntry) (*Project, error) {
	for _, entry := range entries {
		if strings.ToLower(entry.Name()) == "pom.xml" {
			tracing.SetUsageAttributes(fields.AppInitJavaDetect.String("start"))
			pomFile := filepath.Join(path, entry.Name())
			project, err := readMavenProject(pomFile)
			if err != nil {
				return nil, fmt.Errorf("error reading pom.xml: %w", err)
			}

			if len(project.Modules) > 0 {
				// This is a multi-module project, we will capture the analysis, but return nil
				// to continue recursing
				jd.rootProjects = append(jd.rootProjects, *project)
				return nil, nil
			}

			var currentRoot *mavenProject
			for _, rootProject := range jd.rootProjects {
				// we can say that the project is in the root project if the path is under the project
				if inRoot := strings.HasPrefix(pomFile, rootProject.path); inRoot {
					currentRoot = &rootProject
				}
			}

			_ = currentRoot // use currentRoot here in the analysis
			result, err := detectDependencies(project, &Project{
				Language:      Java,
				Path:          path,
				DetectionRule: "Inferred by presence of: pom.xml",
			})
			if err != nil {
				return nil, fmt.Errorf("detecting dependencies: %w", err)
			}

			tracing.SetUsageAttributes(fields.AppInitJavaDetect.String("finish"))
			return result, nil
		}
	}

	return nil, nil
}

// mavenProject represents the top-level structure of a Maven POM file.
type mavenProject struct {
	XmlName              xml.Name             `xml:"project"`
	Parent               parent               `xml:"parent"`
	Modules              []string             `xml:"modules>module"` // Capture the modules
	Dependencies         []dependency         `xml:"dependencies>dependency"`
	DependencyManagement dependencyManagement `xml:"dependencyManagement"`
	Build                build                `xml:"build"`
	path                 string
}

// Parent represents the parent POM if this project is a module.
type parent struct {
	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
}

// Dependency represents a single Maven dependency.
type dependency struct {
	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope,omitempty"`
}

// DependencyManagement includes a list of dependencies that are managed.
type dependencyManagement struct {
	Dependencies []dependency `xml:"dependencies>dependency"`
}

// Build represents the build configuration which can contain plugins.
type build struct {
	Plugins []plugin `xml:"plugins>plugin"`
}

// Plugin represents a build plugin.
type plugin struct {
	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
}

func readMavenProject(filePath string) (*mavenProject, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var project mavenProject
	if err := xml.Unmarshal(bytes, &project); err != nil {
		return nil, fmt.Errorf("parsing xml: %w", err)
	}

	project.path = filepath.Dir(filePath)

	return &project, nil
}

func detectDependencies(mavenProject *mavenProject, project *Project) (*Project, error) {
	// how can we tell it's a Spring Boot project?
	// 1. It has a parent with a groupId of org.springframework.boot and an artifactId of spring-boot-starter-parent
	// 2. It has a dependency with a groupId of org.springframework.boot and an artifactId that starts with
	// spring-boot-starter
	isSpringBoot := false
	if mavenProject.Parent.GroupId == "org.springframework.boot" &&
		mavenProject.Parent.ArtifactId == "spring-boot-starter-parent" {
		isSpringBoot = true
	}
	for _, dep := range mavenProject.Dependencies {
		if dep.GroupId == "org.springframework.boot" && strings.HasPrefix(dep.ArtifactId, "spring-boot-starter") {
			isSpringBoot = true
			break
		}
	}
	applicationProperties := make(map[string]string)
	if isSpringBoot {
		applicationProperties = readProperties(project.Path)
	}

	databaseDepMap := map[DatabaseDep]struct{}{}
	for _, dep := range mavenProject.Dependencies {
		if dep.GroupId == "com.mysql" && dep.ArtifactId == "mysql-connector-j" {
			databaseDepMap[DbMySql] = struct{}{}
		}

		if dep.GroupId == "org.postgresql" && dep.ArtifactId == "postgresql" {
			databaseDepMap[DbPostgres] = struct{}{}
		}

		if dep.GroupId == "com.azure.spring" && dep.ArtifactId == "spring-cloud-azure-starter-data-cosmos" {
			databaseDepMap[DbCosmos] = struct{}{}
		}

		if dep.GroupId == "org.springframework.boot" && dep.ArtifactId == "spring-boot-starter-data-redis" {
			databaseDepMap[DbRedis] = struct{}{}
		}
		if dep.GroupId == "org.springframework.boot" && dep.ArtifactId == "spring-boot-starter-data-redis-reactive" {
			databaseDepMap[DbRedis] = struct{}{}
		}

		if dep.GroupId == "org.springframework.boot" && dep.ArtifactId == "spring-boot-starter-data-mongodb" {
			databaseDepMap[DbMongo] = struct{}{}
		}
		if dep.GroupId == "org.springframework.boot" && dep.ArtifactId == "spring-boot-starter-data-mongodb-reactive" {
			databaseDepMap[DbMongo] = struct{}{}
		}

		if dep.GroupId == "com.azure.spring" && dep.ArtifactId == "spring-cloud-azure-stream-binder-servicebus" {
			bindingDestinations := findBindingDestinations(applicationProperties)
			destinations := make([]string, 0, len(bindingDestinations))
			for bindingName, destination := range bindingDestinations {
				destinations = append(destinations, destination)
				log.Printf("Service Bus queue [%s] found for binding [%s]", destination, bindingName)
			}
			project.AzureDeps = append(project.AzureDeps, AzureDepServiceBus{
				Queues: destinations,
			})
		}

		if dep.GroupId == "com.azure.spring" && dep.ArtifactId == "spring-cloud-azure-stream-binder-eventhubs" {
			bindingDestinations := findBindingDestinations(applicationProperties)
			var destinations []string
			containsInBinding := false
			for bindingName, destination := range bindingDestinations {
				if strings.Contains(bindingName, "-in-") { // Example: consume-in-0
					containsInBinding = true
				}
				if !contains(destinations, destination) {
					destinations = append(destinations, destination)
					log.Printf("Event Hubs [%s] found for binding [%s]", destination, bindingName)
				}
			}
			project.AzureDeps = append(project.AzureDeps, AzureDepEventHubs{
				Names: destinations,
			})
			if containsInBinding {
				project.AzureDeps = append(project.AzureDeps, AzureDepStorageAccount{
					ContainerNames: []string{
						applicationProperties["spring.cloud.azure.eventhubs.processor.checkpoint-store.container-name"]},
				})
			}
		}
	}

	if len(databaseDepMap) > 0 {
		project.DatabaseDeps = slices.SortedFunc(maps.Keys(databaseDepMap),
			func(a, b DatabaseDep) int {
				return strings.Compare(string(a), string(b))
			})
	}

	return project, nil
}

func readProperties(projectPath string) map[string]string {
	// todo: do we need to consider the bootstrap.properties
	result := make(map[string]string)
	readPropertiesInPropertiesFile(filepath.Join(projectPath, "/src/main/resources/application.properties"), result)
	readPropertiesInYamlFile(filepath.Join(projectPath, "/src/main/resources/application.yml"), result)
	readPropertiesInYamlFile(filepath.Join(projectPath, "/src/main/resources/application.yaml"), result)
	profile, profileSet := result["spring.profiles.active"]
	if profileSet {
		readPropertiesInPropertiesFile(
			filepath.Join(projectPath, "/src/main/resources/application-"+profile+".properties"), result)
		readPropertiesInYamlFile(filepath.Join(projectPath, "/src/main/resources/application-"+profile+".yml"), result)
		readPropertiesInYamlFile(filepath.Join(projectPath, "/src/main/resources/application-"+profile+".yaml"), result)
	}
	return result
}

func readPropertiesInYamlFile(yamlFilePath string, result map[string]string) {
	if !osutil.FileExists(yamlFilePath) {
		return
	}
	data, err := os.ReadFile(yamlFilePath)
	if err != nil {
		log.Fatalf("error reading YAML file: %v", err)
		return
	}

	// Parse the YAML into a yaml.Node
	var root yaml.Node
	err = yaml.Unmarshal(data, &root)
	if err != nil {
		log.Fatalf("error unmarshalling YAML: %v", err)
		return
	}

	parseYAML("", &root, result)
}

// Recursively parse the YAML and build dot-separated keys into a map
func parseYAML(prefix string, node *yaml.Node, result map[string]string) {
	switch node.Kind {
	case yaml.DocumentNode:
		// Process each document's content
		for _, contentNode := range node.Content {
			parseYAML(prefix, contentNode, result)
		}
	case yaml.MappingNode:
		// Process key-value pairs in a map
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Ensure the key is a scalar
			if keyNode.Kind != yaml.ScalarNode {
				continue
			}

			keyStr := keyNode.Value
			newPrefix := keyStr
			if prefix != "" {
				newPrefix = prefix + "." + keyStr
			}
			parseYAML(newPrefix, valueNode, result)
		}
	case yaml.SequenceNode:
		// Process items in a sequence (list)
		for i, item := range node.Content {
			newPrefix := fmt.Sprintf("%s[%d]", prefix, i)
			parseYAML(newPrefix, item, result)
		}
	case yaml.ScalarNode:
		// If it's a scalar value, add it to the result map
		result[prefix] = node.Value
	default:
		// Handle other node types if necessary
	}
}

func readPropertiesInPropertiesFile(propertiesFilePath string, result map[string]string) {
	if !osutil.FileExists(propertiesFilePath) {
		return
	}
	file, err := os.Open(propertiesFilePath)
	if err != nil {
		log.Fatalf("error opening properties file: %v", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			result[key] = value
		}
	}
}

// Function to find all properties that match the pattern `spring.cloud.stream.bindings.<binding-name>.destination`
func findBindingDestinations(properties map[string]string) map[string]string {
	result := make(map[string]string)

	// Iterate through the properties map and look for matching keys
	for key, value := range properties {
		// Check if the key matches the pattern `spring.cloud.stream.bindings.<binding-name>.destination`
		if strings.HasPrefix(key, "spring.cloud.stream.bindings.") && strings.HasSuffix(key, ".destination") {
			// Extract the binding name
			bindingName := key[len("spring.cloud.stream.bindings.") : len(key)-len(".destination")]
			// Store the binding name and destination value
			result[bindingName] = fmt.Sprintf("%v", value)
		}
	}

	return result
}

func contains(array []string, str string) bool {
	for _, v := range array {
		if v == str {
			return true
		}
	}
	return false
}
