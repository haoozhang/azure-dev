package appdetect

import (
	"fmt"
	"log"
	"maps"
	"regexp"
	"slices"
	"strings"
)

type SpringBootProject struct {
	springBootVersion     string
	applicationProperties map[string]string
	parentProject         *mavenProject
	mavenProject          *mavenProject
}

type DatabaseDependencyRule struct {
	databaseDep       DatabaseDep
	mavenDependencies []MavenDependency
}

type MavenDependency struct {
	groupId    string
	artifactId string
}

var databaseDependencyRules = []DatabaseDependencyRule{
	{
		databaseDep: DbPostgres,
		mavenDependencies: []MavenDependency{
			{
				groupId:    "org.postgresql",
				artifactId: "postgresql",
			},
		},
	},
	{
		databaseDep: DbMySql,
		mavenDependencies: []MavenDependency{
			{
				groupId:    "com.mysql",
				artifactId: "mysql-connector-j",
			},
		},
	},
	{
		databaseDep: DbRedis,
		mavenDependencies: []MavenDependency{
			{
				groupId:    "org.springframework.boot",
				artifactId: "spring-boot-starter-data-redis",
			},
			{
				groupId:    "org.springframework.boot",
				artifactId: "spring-boot-starter-data-redis-reactive",
			},
		},
	},
	{
		databaseDep: DbMongo,
		mavenDependencies: []MavenDependency{
			{
				groupId:    "org.springframework.boot",
				artifactId: "spring-boot-starter-data-mongodb",
			},
			{
				groupId:    "org.springframework.boot",
				artifactId: "spring-boot-starter-data-mongodb-reactive",
			},
		},
	},
	{
		databaseDep: DbCosmos,
		mavenDependencies: []MavenDependency{
			{
				groupId:    "com.azure.spring",
				artifactId: "spring-cloud-azure-starter-data-cosmos",
			},
		},
	},
}

func detectAzureDependenciesByAnalyzingSpringBootProject(
	parentProject *mavenProject, mavenProject *mavenProject, azdProject *Project) {
	if !isSpringBootApplication(mavenProject) {
		log.Printf("Skip analyzing spring boot project. path = %s.", mavenProject.path)
		return
	}
	var springBootProject = SpringBootProject{
		springBootVersion:     detectSpringBootVersion(parentProject, mavenProject),
		applicationProperties: readProperties(azdProject.Path),
		parentProject:         parentProject,
		mavenProject:          mavenProject,
	}
	detectDatabases(azdProject, &springBootProject)
	detectServiceBus(azdProject, &springBootProject)
	detectEventHubs(azdProject, &springBootProject)
	detectStorageAccount(azdProject, &springBootProject)
	detectMetadata(azdProject, &springBootProject)
	detectSpringFrontend(azdProject, &springBootProject)
}

func detectSpringFrontend(azdProject *Project, springBootProject *SpringBootProject) {
	for _, p := range springBootProject.mavenProject.Build.Plugins {
		if p.GroupId == "com.github.eirslett" && p.ArtifactId == "frontend-maven-plugin" {
			azdProject.Dependencies = append(azdProject.Dependencies, SpringFrontend)
			break
		}
	}
}

func detectDatabases(azdProject *Project, springBootProject *SpringBootProject) {
	databaseDepMap := map[DatabaseDep]struct{}{}
	for _, rule := range databaseDependencyRules {
		for _, targetDependency := range rule.mavenDependencies {
			var targetGroupId = targetDependency.groupId
			var targetArtifactId = targetDependency.artifactId
			if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
				databaseDepMap[rule.databaseDep] = struct{}{}
				logServiceAddedAccordingToMavenDependency(rule.databaseDep.Display(),
					targetGroupId, targetArtifactId)
				break
			}
		}
	}
	if len(databaseDepMap) > 0 {
		azdProject.DatabaseDeps = slices.SortedFunc(maps.Keys(databaseDepMap),
			func(a, b DatabaseDep) int {
				return strings.Compare(string(a), string(b))
			})
	}
}

func detectServiceBus(azdProject *Project, springBootProject *SpringBootProject) {
	// we need to figure out multiple projects are using the same service bus
	detectServiceBusAccordingToJMSMavenDependency(azdProject, springBootProject)
	detectServiceBusAccordingToSpringCloudStreamBinderMavenDependency(azdProject, springBootProject)
}

func detectServiceBusAccordingToJMSMavenDependency(azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "com.azure.spring"
	var targetArtifactId = "spring-cloud-azure-starter-servicebus-jms"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		newDependency := AzureDepServiceBus{
			IsJms: true,
		}
		azdProject.AzureDeps = append(azdProject.AzureDeps, newDependency)
		logServiceAddedAccordingToMavenDependency(newDependency.ResourceDisplay(), targetGroupId, targetArtifactId)
	}
}

func detectServiceBusAccordingToSpringCloudStreamBinderMavenDependency(
	azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "com.azure.spring"
	var targetArtifactId = "spring-cloud-azure-stream-binder-servicebus"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		bindingDestinations := getBindingDestinationMap(springBootProject.applicationProperties)
		var destinations = distinctValues(bindingDestinations)
		newDep := AzureDepServiceBus{
			Queues: destinations,
			IsJms:  false,
		}
		azdProject.AzureDeps = append(azdProject.AzureDeps, newDep)
		logServiceAddedAccordingToMavenDependency(newDep.ResourceDisplay(), targetGroupId, targetArtifactId)
		for bindingName, destination := range bindingDestinations {
			log.Printf("  Detected Service Bus queue [%s] for binding [%s] by analyzing property file.",
				destination, bindingName)
		}
	}
}

func detectEventHubs(azdProject *Project, springBootProject *SpringBootProject) {
	// we need to figure out multiple projects are using the same event hub
	detectEventHubsAccordingToSpringCloudStreamBinderMavenDependency(azdProject, springBootProject)
	detectEventHubsAccordingToSpringCloudStreamKafkaMavenDependency(azdProject, springBootProject)
}

func detectEventHubsAccordingToSpringCloudStreamBinderMavenDependency(
	azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "com.azure.spring"
	var targetArtifactId = "spring-cloud-azure-stream-binder-eventhubs"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		bindingDestinations := getBindingDestinationMap(springBootProject.applicationProperties)
		var destinations = distinctValues(bindingDestinations)
		newDep := AzureDepEventHubs{
			Names:    destinations,
			UseKafka: false,
		}
		azdProject.AzureDeps = append(azdProject.AzureDeps, newDep)
		logServiceAddedAccordingToMavenDependency(newDep.ResourceDisplay(), targetGroupId, targetArtifactId)
		for bindingName, destination := range bindingDestinations {
			log.Printf("  Detected Event Hub [%s] for binding [%s] by analyzing property file.",
				destination, bindingName)
		}
	}
}

func detectEventHubsAccordingToSpringCloudStreamKafkaMavenDependency(
	azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "org.springframework.cloud"
	var targetArtifactId = "spring-cloud-starter-stream-kafka"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		bindingDestinations := getBindingDestinationMap(springBootProject.applicationProperties)
		var destinations = distinctValues(bindingDestinations)
		newDep := AzureDepEventHubs{
			Names:             destinations,
			UseKafka:          true,
			SpringBootVersion: springBootProject.springBootVersion,
		}
		azdProject.AzureDeps = append(azdProject.AzureDeps, newDep)
		logServiceAddedAccordingToMavenDependency(newDep.ResourceDisplay(), targetGroupId, targetArtifactId)
		for bindingName, destination := range bindingDestinations {
			log.Printf("  Detected Kafka Topic [%s] for binding [%s] by analyzing property file.",
				destination, bindingName)
		}
	}
}

func detectStorageAccount(azdProject *Project, springBootProject *SpringBootProject) {
	detectStorageAccountAccordingToSpringCloudStreamBinderMavenDependencyAndProperty(azdProject, springBootProject)
}

func detectStorageAccountAccordingToSpringCloudStreamBinderMavenDependencyAndProperty(
	azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "com.azure.spring"
	var targetArtifactId = "spring-cloud-azure-stream-binder-eventhubs"
	var targetPropertyName = "spring.cloud.azure.eventhubs.processor.checkpoint-store.container-name"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		bindingDestinations := getBindingDestinationMap(springBootProject.applicationProperties)
		containsInBindingName := ""
		for bindingName := range bindingDestinations {
			if strings.Contains(bindingName, "-in-") { // Example: consume-in-0
				containsInBindingName = bindingName
				break
			}
		}
		if containsInBindingName != "" {
			targetPropertyValue := springBootProject.applicationProperties[targetPropertyName]
			newDep := AzureDepStorageAccount{
				ContainerNames: []string{targetPropertyValue},
			}
			azdProject.AzureDeps = append(azdProject.AzureDeps, newDep)
			logServiceAddedAccordingToMavenDependencyAndExtraCondition(newDep.ResourceDisplay(), targetGroupId,
				targetArtifactId, "binding name ["+containsInBindingName+"] contains '-in-'")
			log.Printf("  Detected Storage Account container name: [%s] by analyzing property file.",
				targetPropertyValue)
		}
	}
}

func detectMetadata(azdProject *Project, springBootProject *SpringBootProject) {
	detectPropertySpringApplicationName(azdProject, springBootProject)
	detectPropertySpringDatasourceUrl(azdProject, springBootProject)
	detectDependencySpringCloudAzureStarter(azdProject, springBootProject)
	detectDependencySpringCloudAzureStarterJdbcPostgresql(azdProject, springBootProject)
	detectDependencySpringCloudAzureStarterJdbcMysql(azdProject, springBootProject)
	detectDependencySpringCloudEureka(azdProject, springBootProject)
	detectDependencySpringCloudConfig(azdProject, springBootProject)
}

func detectPropertySpringDatasourceUrl(azdProject *Project, springBootProject *SpringBootProject) {
	var targetPropertyName = "spring.datasource.url"
	propertyValue, ok := springBootProject.applicationProperties[targetPropertyName]
	if !ok {
		log.Printf("spring.datasource.url property not exist in project. Path = %s", azdProject.Path)
		return
	}
	databaseName := getDatabaseName(propertyValue)
	if databaseName == "" {
		log.Printf("can not get database name from property: spring.datasource.url")
		return
	}
	if azdProject.Metadata.DatabaseNameInPropertySpringDatasourceUrl == nil {
		azdProject.Metadata.DatabaseNameInPropertySpringDatasourceUrl = map[DatabaseDep]string{}
	}
	if strings.HasPrefix(propertyValue, "jdbc:postgresql") {
		azdProject.Metadata.DatabaseNameInPropertySpringDatasourceUrl[DbPostgres] = databaseName
	} else if strings.HasPrefix(propertyValue, "jdbc:mysql") {
		azdProject.Metadata.DatabaseNameInPropertySpringDatasourceUrl[DbMySql] = databaseName
	}
}

func getDatabaseName(datasourceURL string) string {
	lastSlashIndex := strings.LastIndex(datasourceURL, "/")
	if lastSlashIndex == -1 {
		return ""
	}
	result := datasourceURL[lastSlashIndex+1:]
	if idx := strings.Index(result, "?"); idx != -1 {
		result = result[:idx]
	}
	if IsValidDatabaseName(result) {
		return result
	}
	return ""
}

func IsValidDatabaseName(name string) bool {
	if len(name) < 3 || len(name) > 63 {
		return false
	}
	re := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	return re.MatchString(name)
}

func detectDependencySpringCloudAzureStarter(azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "com.azure.spring"
	var targetArtifactId = "spring-cloud-azure-starter"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		azdProject.Metadata.ContainsDependencySpringCloudAzureStarter = true
		logMetadataUpdated("ContainsDependencySpringCloudAzureStarter = true")
	}
}

func detectDependencySpringCloudAzureStarterJdbcPostgresql(azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "com.azure.spring"
	var targetArtifactId = "spring-cloud-azure-starter-jdbc-postgresql"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		azdProject.Metadata.ContainsDependencySpringCloudAzureStarterJdbcPostgresql = true
		logMetadataUpdated("ContainsDependencySpringCloudAzureStarterJdbcPostgresql = true")
	}
}

func detectDependencySpringCloudAzureStarterJdbcMysql(azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "com.azure.spring"
	var targetArtifactId = "spring-cloud-azure-starter-jdbc-mysql"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		azdProject.Metadata.ContainsDependencySpringCloudAzureStarterJdbcMysql = true
		logMetadataUpdated("ContainsDependencySpringCloudAzureStarterJdbcMysql = true")
	}
}

func detectPropertySpringApplicationName(azdProject *Project, springBootProject *SpringBootProject) {
	var targetPropertyName = "spring.application.name"
	if appName, ok := springBootProject.applicationProperties[targetPropertyName]; ok {
		azdProject.Metadata.ApplicationName = appName
	}
}

func detectDependencySpringCloudEureka(azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "org.springframework.cloud"
	var targetArtifactId = "spring-cloud-starter-netflix-eureka-server"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		azdProject.Metadata.ContainsDependencySpringCloudEurekaServer = true
		logMetadataUpdated("ContainsDependencySpringCloudEurekaServer = true")
	}

	targetGroupId = "org.springframework.cloud"
	targetArtifactId = "spring-cloud-starter-netflix-eureka-client"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		azdProject.Metadata.ContainsDependencySpringCloudEurekaClient = true
		logMetadataUpdated("ContainsDependencySpringCloudEurekaClient = true")
	}
}

func detectDependencySpringCloudConfig(azdProject *Project, springBootProject *SpringBootProject) {
	var targetGroupId = "org.springframework.cloud"
	var targetArtifactId = "spring-cloud-config-server"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		azdProject.Metadata.ContainsDependencySpringCloudConfigServer = true
		logMetadataUpdated("ContainsDependencySpringCloudConfigServer = true")
	}

	targetGroupId = "org.springframework.cloud"
	targetArtifactId = "spring-cloud-starter-config"
	if hasDependency(springBootProject, targetGroupId, targetArtifactId) {
		azdProject.Metadata.ContainsDependencySpringCloudConfigClient = true
		logMetadataUpdated("ContainsDependencySpringCloudConfigClient = true")
	}
}

func logServiceAddedAccordingToMavenDependency(resourceName, groupId string, artifactId string) {
	logServiceAddedAccordingToMavenDependencyAndExtraCondition(resourceName, groupId, artifactId, "")
}

func logServiceAddedAccordingToMavenDependencyAndExtraCondition(
	resourceName, groupId string, artifactId string, extraCondition string) {
	insertedString := ""
	extraCondition = strings.TrimSpace(extraCondition)
	if extraCondition != "" {
		insertedString = " and " + extraCondition
	}
	log.Printf("Detected '%s' because found dependency '%s:%s' in pom.xml file%s.",
		resourceName, groupId, artifactId, insertedString)
}

func logMetadataUpdated(info string) {
	log.Printf("Metadata updated. %s.", info)
}

func detectSpringBootVersion(currentRoot *mavenProject, mavenProject *mavenProject) string {
	// mavenProject prioritize than rootProject
	if mavenProject != nil {
		if version := detectSpringBootVersionFromProject(mavenProject); version != UnknownSpringBootVersion {
			return version
		}
	}
	// fallback to detect root project
	if currentRoot != nil {
		return detectSpringBootVersionFromProject(currentRoot)
	}
	return UnknownSpringBootVersion
}

func detectSpringBootVersionFromProject(project *mavenProject) string {
	if project.Parent.ArtifactId == "spring-boot-starter-parent" {
		return project.Parent.Version
	} else {
		for _, dep := range project.DependencyManagement.Dependencies {
			if dep.ArtifactId == "spring-boot-dependencies" {
				return dep.Version
			}
		}
	}
	return UnknownSpringBootVersion
}

func isSpringBootApplication(mavenProject *mavenProject) bool {
	// how can we tell it's a Spring Boot project?
	// 1. It has a parent with a groupId of org.springframework.boot and an artifactId of spring-boot-starter-parent
	// 2. It has a dependency with a groupId of org.springframework.boot and an artifactId that starts with
	// spring-boot-starter
	if mavenProject.Parent.GroupId == "org.springframework.boot" &&
		mavenProject.Parent.ArtifactId == "spring-boot-starter-parent" {
		return true
	}
	for _, dep := range mavenProject.Dependencies {
		if dep.GroupId == "org.springframework.boot" &&
			strings.HasPrefix(dep.ArtifactId, "spring-boot-starter") {
			return true
		}
	}
	return false
}

func distinctValues(input map[string]string) []string {
	valueSet := make(map[string]struct{})
	for _, value := range input {
		valueSet[value] = struct{}{}
	}

	var result []string
	for value := range valueSet {
		result = append(result, value)
	}

	return result
}

// Function to find all properties that match the pattern `spring.cloud.stream.bindings.<binding-name>.destination`
func getBindingDestinationMap(properties map[string]string) map[string]string {
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

func hasDependency(project *SpringBootProject, groupId string, artifactId string) bool {
	for _, projectDependency := range project.mavenProject.Dependencies {
		if projectDependency.GroupId == groupId && projectDependency.ArtifactId == artifactId {
			return true
		}
	}
	return false
}
