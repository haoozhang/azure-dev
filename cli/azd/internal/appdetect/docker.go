package appdetect

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func detectDockerInDirectory(project Project, entries []fs.DirEntry) (*Docker, error) {
	path := project.Path
	for _, entry := range entries {
		if strings.ToLower(entry.Name()) == "dockerfile" {
			dockerFilePath := filepath.Join(path, entry.Name())
			return AnalyzeDocker(dockerFilePath)
		}
	}

	// if Dockerfile not exists, provide a default one
	if project.Language == Java {
		_, hasParentPom := project.Options[JavaProjectOptionMavenParentPath]
		err := addDefaultDockerfile(path, hasParentPom)
		if err != nil {
			return nil, err
		}
		dockerfilePath := filepath.Join(path, "Dockerfile")
		return AnalyzeDocker(dockerfilePath)
	}

	return nil, nil
}

// AnalyzeDocker analyzes the Dockerfile and returns the Docker result.
func AnalyzeDocker(dockerFilePath string) (*Docker, error) {
	file, err := os.Open(dockerFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading Dockerfile at %s: %w", dockerFilePath, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	var ports []Port
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "EXPOSE") {
			parsedPorts, err := parsePortsInLine(line[len("EXPOSE"):])
			if err != nil {
				log.Printf("parsing Dockerfile at %s: %v", dockerFilePath, err)
			}
			ports = append(ports, parsedPorts...)
		}
	}
	return &Docker{
		Path:  dockerFilePath,
		Ports: ports,
	}, nil
}

func parsePortsInLine(s string) ([]Port, error) {
	var ports []Port
	portSpecs := strings.Fields(s)
	for _, portSpec := range portSpecs {
		var portString string
		var protocol string
		if strings.Contains(portSpec, "/") {
			parts := strings.Split(portSpec, "/")
			portString = parts[0]
			protocol = parts[1]
		} else {
			portString = portSpec
			protocol = "tcp"
		}
		portNumber, err := strconv.Atoi(portString)
		if err != nil {
			return nil, fmt.Errorf("parsing port number: %w", err)
		}
		ports = append(ports, Port{portNumber, protocol})
	}
	return ports, nil
}

const (
	DockerfileSingleStage = `FROM openjdk:17-jdk-slim
COPY ./target/*.jar app.jar
COPY ./target/*.war app.war
ENTRYPOINT ["sh", "-c", \
    "if [ -f /app.jar ]; then java -jar /app.jar; \
    elif [ -f /app.war ]; then java -jar /app.war; \
    else echo 'No JAR or WAR file found'; fi"]`

	DockerfileMultiStage = `FROM maven:3 AS build
WORKDIR /app
COPY . .
RUN mvn --batch-mode clean package -DskipTests

FROM openjdk:17-jdk-slim
WORKDIR /
COPY --from=build /app/target/*.jar app.jar
COPY --from=build /app/target/*.war app.war
ENTRYPOINT ["sh", "-c", \
    "if [ -f /app.jar ]; then java -jar /app.jar; \
    elif [ -f /app.war ]; then java -jar /app.war; \
    else echo 'No JAR or WAR file found'; fi"]`
)

func addDefaultDockerfile(path string, hasParentPom bool) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("error accessing path %s: %w", path, err)
	}

	dockerfilePath := filepath.Join(path, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err == nil {
		fmt.Println("Dockerfile already exists, skipping creation.")
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error checking Dockerfile at path %s: %w", path, err)
	}

	file, err := os.Create(dockerfilePath)
	if err != nil {
		return fmt.Errorf("failed to create Dockerfile at %s: %w", dockerfilePath, err)
	}
	defer file.Close()

	// for single-module project, we have to run 'mvn package' first, then copy and run jar
	// for multi-module project, just copy and run jar because 'mvn package' already executed in prepackage hook
	var dockerfileContent string
	if hasParentPom {
		dockerfileContent = DockerfileSingleStage
	} else {
		dockerfileContent = DockerfileMultiStage
	}

	if _, err = file.WriteString(dockerfileContent); err != nil {
		return fmt.Errorf("failed to write Dockerfile at %s: %w", dockerfilePath, err)
	}

	return nil
}
