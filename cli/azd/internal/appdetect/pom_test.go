package appdetect

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceAllPlaceholders(t *testing.T) {
	tests := []struct {
		name   string
		pom    pom
		input  string
		output string
	}{
		{
			"empty.input",
			pom{
				Properties: Properties{
					Entries: []Property{
						{
							XMLName: xml.Name{
								Local: "version.spring-boot_2.x",
							},
							Value: "2.x",
						},
					},
				},
			},
			"",
			"",
		},
		{
			"empty.properties",
			pom{
				Properties: Properties{
					Entries: []Property{},
				},
			},
			"org.springframework.boot:spring-boot-dependencies:${version.spring-boot_2.x}",
			"org.springframework.boot:spring-boot-dependencies:${version.spring-boot_2.x}",
		},
		{
			"dependency.version",
			pom{
				Properties: Properties{
					Entries: []Property{
						{
							XMLName: xml.Name{
								Local: "version.spring-boot_2.x",
							},
							Value: "2.x",
						},
					},
				},
			},
			"org.springframework.boot:spring-boot-dependencies:${version.spring-boot_2.x}",
			"org.springframework.boot:spring-boot-dependencies:2.x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := replaceAllPlaceholders(tt.pom, tt.input)
			assert.Equal(t, tt.output, output)
		})
	}
}

func TestToEffectivePom(t *testing.T) {
	tests := []struct {
		name       string
		pomContent string
		expected   []dependency
	}{
		{
			name: "Test with two dependencies",
			pomContent: `
				<project>
					<modelVersion>4.0.0</modelVersion>
					<groupId>com.example</groupId>
					<artifactId>example-project</artifactId>
					<version>1.0.0</version>
					<dependencies>
						<dependency>
							<groupId>org.springframework</groupId>
							<artifactId>spring-core</artifactId>
							<version>5.3.8</version>
							<scope>compile</scope>
						</dependency>
						<dependency>
							<groupId>junit</groupId>
							<artifactId>junit</artifactId>
							<version>4.13.2</version>
							<scope>test</scope>
						</dependency>
					</dependencies>
				</project>
				`,
			expected: []dependency{
				{
					GroupId:    "org.springframework",
					ArtifactId: "spring-core",
					Version:    "5.3.8",
					Scope:      "compile",
				},
				{
					GroupId:    "junit",
					ArtifactId: "junit",
					Version:    "4.13.2",
					Scope:      "test",
				},
			},
		},
		{
			name: "Test with no dependencies",
			pomContent: `
				<project>
					<modelVersion>4.0.0</modelVersion>
					<groupId>com.example</groupId>
					<artifactId>example-project</artifactId>
					<version>1.0.0</version>
					<dependencies>
					</dependencies>
				</project>
				`,
			expected: []dependency{},
		},
		{
			name: "Test with one dependency which version is decided by dependencyManagement",
			pomContent: `
				<project>
					<modelVersion>4.0.0</modelVersion>
					<groupId>com.example</groupId>
					<artifactId>example-project</artifactId>
					<version>1.0.0</version>
					<dependencies>
						<dependency>
							<groupId>org.slf4j</groupId>
							<artifactId>slf4j-api</artifactId>
						</dependency>
					</dependencies>
					<dependencyManagement>
						<dependencies>
							<dependency>
								<groupId>org.springframework.boot</groupId>
								<artifactId>spring-boot-dependencies</artifactId>
								<version>3.0.0</version>
								<type>pom</type>
								<scope>import</scope>
							</dependency>
						</dependencies>
					</dependencyManagement>
				</project>
				`,
			expected: []dependency{
				{
					GroupId:    "org.slf4j",
					ArtifactId: "slf4j-api",
					Version:    "2.0.4",
					Scope:      "compile",
				},
			},
		},
		{
			name: "Test with one dependency which version is decided by parent",
			pomContent: `
				<project>
					<parent>
						<groupId>org.springframework.boot</groupId>
						<artifactId>spring-boot-starter-parent</artifactId>
						<version>3.0.0</version>
						<relativePath/> <!-- lookup parent from repository -->
					</parent>
					<modelVersion>4.0.0</modelVersion>
					<groupId>com.example</groupId>
					<artifactId>example-project</artifactId>
					<version>1.0.0</version>
					<dependencies>
						<dependency>
							<groupId>org.slf4j</groupId>
							<artifactId>slf4j-api</artifactId>
						</dependency>
					</dependencies>
				</project>
				`,
			expected: []dependency{
				{
					GroupId:    "org.slf4j",
					ArtifactId: "slf4j-api",
					Version:    "2.0.4",
					Scope:      "compile",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "test")
			if err != nil {
				t.Fatalf("Failed to create temp directory: %v", err)
			}
			defer func(path string) {
				err := os.RemoveAll(path)
				if err != nil {
					t.Fatalf("Failed to remove all in directory: %v", err)
				}
			}(tempDir)

			pomPath := filepath.Join(tempDir, "pom.xml")
			err = os.WriteFile(pomPath, []byte(tt.pomContent), 0600)
			if err != nil {
				t.Fatalf("Failed to write temp POM file: %v", err)
			}

			effectivePom, err := toEffectivePom(pomPath)
			if err != nil {
				t.Fatalf("toEffectivePom failed: %v", err)
			}

			if len(effectivePom.Dependencies) != len(tt.expected) {
				t.Fatalf("Expected %d dependencies, got %d", len(tt.expected), len(effectivePom.Dependencies))
			}

			for i, dep := range effectivePom.Dependencies {
				if dep != tt.expected[i] {
					t.Errorf("Expected dependency %v, got %v", tt.expected[i], dep)
				}
			}
		})
	}
}