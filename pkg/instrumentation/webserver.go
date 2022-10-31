// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package instrumentation

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
)

const (
	nginxConfigDirectory         = "/etc/nginx"
	nginxConfigFile              = "nginx.conf"
	nginxAgentInitContainerName  = "otel-agent-attach-nginx"
	nginxAgentCloneContainerName = "otel-agent-source-container-clone"
	nginxAgentConfigVolume       = "otel-nginx-conf-dir"
	nginxAgentVolume             = "otel-nginx-agent"
	nginxAttributesEnvVar        = "OTEL_NGINX_AGENT_CONF"
	nginxServiceInstanceIdEnvVar = "NGINX_SERVICE_INSTANCE_ID"
)

const (
	apacheConfigDirectory         = "/usr/local/apache2/conf"
	apacheConfigFile              = "httpd.conf"
	apacheAgentInitContainerName  = "otel-agent-attach-apache"
	apacheAgentCloneContainerName = "otel-agent-source-container-clone"
	apacheAgentConfigVolume       = "otel-apache-conf-dir"
	apacheAgentVolume             = "otel-apache-agent"
	apacheAttributesEnvVar        = "OTEL_APACHE_AGENT_CONF"
	apacheServiceInstanceIdEnvVar = "APACHE_SERVICE_INSTANCE_ID"
)

type webserver string

const (
	// apache webserver type.
	apache webserver = "apache"
	// nginx webserver type.
	nginx webserver = "nginx"
)

const (
	webserverAgentConfigFile      = "opentelemetry_module.conf"
	webserverAgentDirectory       = "/opt/opentelemetry-webserver"
	webserverAgentSubDirectory    = "/agent"
	webserverAgentConfigDirectory = "/source-conf"
	webserverAgentDirFull         = webserverAgentDirectory + webserverAgentSubDirectory
	webserverAgentConfDirFull     = webserverAgentDirectory + webserverAgentConfigDirectory
	webserverServiceInstanceId    = "<<SID-PLACEHOLDER>>"

	libLibraryPath                = "LD_LIBRARY_PATH"
	webserverAgentLibSubDirectory = "/sdk_lib/lib"
)

var (
	webserverConfigDirectory         string
	webserverConfigFile              string
	webserverAgentInitContainerName  string
	webserverAgentCloneContainerName string
	webserverAgentConfigVolume       string
	webserverAgentVolume             string
	webserverAttributesEnvVar        string
	webserverServiceInstanceIdEnvVar string
)

func initVar(webserverType webserver) {
	switch webserverType {
	case apache:
		webserverConfigDirectory = apacheConfigDirectory
		webserverConfigFile = apacheConfigFile
		webserverAgentInitContainerName = apacheAgentInitContainerName
		webserverAgentCloneContainerName = apacheAgentCloneContainerName
		webserverAgentConfigVolume = apacheAgentConfigVolume
		webserverAgentVolume = apacheAgentVolume
		webserverAttributesEnvVar = apacheAttributesEnvVar
		webserverServiceInstanceIdEnvVar = apacheServiceInstanceIdEnvVar
	case nginx:
		webserverConfigDirectory = nginxConfigDirectory
		webserverConfigFile = nginxConfigFile
		webserverAgentInitContainerName = nginxAgentInitContainerName
		webserverAgentCloneContainerName = nginxAgentCloneContainerName
		webserverAgentConfigVolume = nginxAgentConfigVolume
		webserverAgentVolume = nginxAgentVolume
		webserverAttributesEnvVar = nginxAttributesEnvVar
		webserverServiceInstanceIdEnvVar = nginxServiceInstanceIdEnvVar
	default:
	}
}

func injectWebserverAgent(webserverSpec v1alpha1.Webserver, pod corev1.Pod, index int, otlpEndpoint string, resourceMap map[string]string, webserverType webserver) corev1.Pod {

	initVar(webserverType)
	// caller checks if there is at least one container
	container := &pod.Spec.Containers[index]

	// inject env vars
	for _, env := range webserverSpec.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
	}

	// First make a clone of the instrumented container to take the existing Apache configuration from
	if isApacheInitContainerMissing(pod, webserverAgentCloneContainerName) {
		// Inject volume for original Apache configuration
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: webserverAgentConfigVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			}})

		cloneContainer := container.DeepCopy()
		cloneContainer.Name = webserverAgentCloneContainerName
		cloneContainer.Command = []string{"/bin/sh", "-c"}
		cloneContainer.Args = []string{"cp -r " + webserverConfigDirectory + "/* " + webserverAgentConfDirFull}

		cloneContainer.VolumeMounts = append(cloneContainer.VolumeMounts, corev1.VolumeMount{
			Name:      webserverAgentConfigVolume,
			MountPath: webserverAgentConfDirFull,
		})
		// remove resource requirements since those are then reserved for the lifetime of a pod
		// and we definitely do not need them for the init container for cp command
		cloneContainer.Resources = corev1.ResourceRequirements{}

		pod.Spec.InitContainers = append(pod.Spec.InitContainers, *cloneContainer)

		// drop volume mount with volume-provided Apache config from original container
		// since it could over-write configuration provided by the injection
		idxFound := -1
		for idx, volume := range container.VolumeMounts {
			if strings.Contains(volume.MountPath, webserverConfigDirectory) { // potentially passes config, which we want to pass to init copy only
				idxFound = idx
				break
			}
		}
		if idxFound >= 0 {
			volumeMounts := container.VolumeMounts
			volumeMounts = append(volumeMounts[:idxFound], volumeMounts[idxFound+1:]...)
			container.VolumeMounts = volumeMounts
		}

		// Inject volumes info instrumented container - Apache config dir + Apache agent
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      webserverAgentVolume,
			MountPath: webserverAgentDirFull,
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      webserverAgentConfigVolume,
			MountPath: webserverConfigDirectory,
		})
	}

	// We just inject Volumes and init containers for the first processed container
	if isApacheInitContainerMissing(pod, webserverAgentInitContainerName) {
		// Inject volume for agent
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: webserverAgentVolume,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			}})

		var initContainersArgs string
		if webserverType == nginx {
			env := corev1.EnvVar{
				Name:  libLibraryPath,
				Value: webserverAgentDirFull + webserverAgentLibSubDirectory,
			}
			// inject env vars
			if idx := getIndexOfEnv(container.Env, env.Name); idx == -1 {
				container.Env = append(container.Env, env)
			} else {
				container.Env[idx].Value = ":" + env.Value
			}

			initContainersArgs = " && " +
				// loads module for Nginx webserver
				"echo \"load_module " + webserverAgentDirFull + "/WebServerModule/Nginx/ngx_http_opentelemetry_module.so; $(cat " + webserverConfigDirectory + "/" + webserverConfigFile + ")\" > " + webserverConfigDirectory + "/" + webserverConfigFile + " && " +
				// Copy the otel conf to conf.d directory
				"mkdir -p " + webserverConfigDirectory + "/" + "conf.d && cp " + webserverConfigDirectory + "/" + webserverAgentConfigFile + " " + webserverConfigDirectory + "/" + "conf.d"
		} else if webserverType == apache {
			initContainersArgs = " && " +
				// Include a link to include Apache agent configuration file into httpd.conf
				"echo 'Include " + webserverConfigDirectory + "/" + webserverAgentConfigFile + "' >> " + webserverAgentConfDirFull + "/" + webserverConfigFile

		}
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:    webserverAgentInitContainerName,
			Image:   webserverSpec.Image,
			Command: []string{"/bin/sh", "-c"},
			Args: []string{
				// Copy agent binaries to shared volume
				"cp -ar /opt/opentelemetry/* " + webserverAgentDirFull + " && " +
					// setup logging configuration from template
					"export agentLogDir=$(echo \"" + webserverAgentDirFull + "/logs\" | sed 's,/,\\\\/,g') && " +
					"cat " + webserverAgentDirFull + "/conf/appdynamics_sdk_log4cxx.xml.template | sed 's/__agent_log_dir__/'${agentLogDir}'/g'  > " + webserverAgentDirFull + "/conf/appdynamics_sdk_log4cxx.xml && " +
					// Create agent configuration file by pasting content of env var to a file
					"echo \"$" + webserverAttributesEnvVar + "\" > " + webserverAgentConfDirFull + "/" + webserverAgentConfigFile + " && " +
					"sed -i 's/" + webserverServiceInstanceId + "/'${" + webserverServiceInstanceIdEnvVar + "}'/g' " + webserverAgentConfDirFull + "/" + webserverAgentConfigFile +
					initContainersArgs,
			},
			Env: []corev1.EnvVar{
				{
					Name:  webserverAttributesEnvVar,
					Value: getWebserverOtelConfig(pod, webserverSpec, index, otlpEndpoint, resourceMap, webserverType),
				},
				{
					Name: webserverServiceInstanceIdEnvVar,
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.name",
						},
					},
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      webserverAgentVolume,
					MountPath: webserverAgentDirFull,
				},
				{
					Name:      webserverAgentConfigVolume,
					MountPath: webserverAgentConfDirFull,
				},
				{
					Name:      webserverAgentConfigVolume,
					MountPath: webserverConfigDirectory,
				},
			},
		})
	}

	return pod
}

// Calculate if we already inject InitContainers.
func isApacheInitContainerMissing(pod corev1.Pod, containerName string) bool {
	for _, initContainer := range pod.Spec.InitContainers {
		if initContainer.Name == containerName {
			return false
		}
	}
	return true
}

// Construct the config file contents based on the webserver type.
func getWebserverConfigFileContent(webserverType webserver, versionSuffix string) string {
	if webserverType == apache {
		template := `
#Load the Otel Webserver SDK
LoadFile %[1]s/sdk_lib/lib/libopentelemetry_common.so
LoadFile %[1]s/sdk_lib/lib/libopentelemetry_resources.so
LoadFile %[1]s/sdk_lib/lib/libopentelemetry_trace.so
LoadFile %[1]s/sdk_lib/lib/libopentelemetry_otlp_recordable.so
LoadFile %[1]s/sdk_lib/lib/libopentelemetry_exporter_ostream_span.so
LoadFile %[1]s/sdk_lib/lib/libopentelemetry_exporter_otlp_grpc.so
#Load the Otel ApacheModule SDK
LoadFile %[1]s/sdk_lib/lib/libopentelemetry_webserver_sdk.so
#Load the Apache Module. In this example for Apache 2.4
#LoadModule otel_apache_module %[1]s/WebServerModule/Apache/libmod_apache_otel.so
#Load the Apache Module. In this example for Apache 2.2
#LoadModule otel_apache_module %[1]s/WebServerModule/Apache/libmod_apache_otel22.so
LoadModule otel_apache_module %[1]s/WebServerModule/Apache/libmod_apache_otel%[2]s.so
#Attributes
`
		return fmt.Sprintf(template,
			webserverAgentDirectory+webserverAgentSubDirectory,
			versionSuffix)
	}
	return ""

}

func getWebserverAttrMap(webserverType webserver, otelEndpoint, serviceName, serviceNamespace string) map[string]string {
	switch webserverType {
	case apache:
		return map[string]string{
			"ApacheModuleEnabled": "ON",
			// ApacheModule Otel Exporter details
			"ApacheModuleOtelSpanExporter":     "otlp",
			"ApacheModuleOtelExporterEndpoint": otelEndpoint,
			// Service name and other IDs
			"ApacheModuleServiceName":       serviceName,
			"ApacheModuleServiceNamespace":  serviceNamespace,
			"ApacheModuleServiceInstanceId": webserverServiceInstanceId,

			"ApacheModuleResolveBackends": " ON",
			"ApacheModuleTraceAsError":    " ON",
		}
	case nginx:
		return map[string]string{
			"NginxModuleEnabled": "ON;",
			// ApacheModule Otel Exporter details
			"NginxModuleOtelSpanExporter":     "otlp;",
			"NginxModuleOtelExporterEndpoint": otelEndpoint + ";",
			// Service name and other IDs
			"NginxModuleServiceName":       serviceName + ";",
			"NginxModuleServiceNamespace":  serviceNamespace + ";",
			"NginxModuleServiceInstanceId": webserverServiceInstanceId + ";",

			"NginxModuleResolveBackends": "ON;",
			"NginxModuleTraceAsError":    "ON;",
		}
	default:
		return map[string]string{}
	}
}

// Calculate Apache/Nginx agent configuration file based on attributes provided by the injection rules
// and by the pod values.
func getWebserverOtelConfig(pod corev1.Pod, webserverSpec v1alpha1.Webserver, index int, otelEndpoint string, resourceMap map[string]string, webserverType webserver) string {

	if otelEndpoint == "" {
		otelEndpoint = "http://localhost:4317/"
	}
	serviceName := chooseServiceName(pod, resourceMap, index)
	serviceNamespace := pod.GetNamespace()
	if annotNamespace, found := pod.GetAnnotations()[annotationInjectOtelNamespace]; found {
		serviceNamespace = annotNamespace
	}

	versionSuffix := ""
	if webserverSpec.Version == "2.2" && webserverType == apache {
		versionSuffix = "22"
	}

	attrMap := getWebserverAttrMap(webserverType, otelEndpoint, serviceName, serviceNamespace)
	for _, attr := range webserverSpec.Attrs {
		attrMap[attr.Name] = attr.Value
	}

	configFileContent := getWebserverConfigFileContent(webserverType, versionSuffix)

	keys := make([]string, 0, len(attrMap))
	for key := range attrMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		configFileContent += fmt.Sprintf("%s %s\n", key, attrMap[key])
	}

	return configFileContent
}
