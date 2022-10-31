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
	"testing"

	"github.com/stretchr/testify/assert"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
)

func TestInjectWebserverAgent(t *testing.T) {
	tests := []struct {
		name string
		v1alpha1.Webserver
		pod           corev1.Pod
		expected      corev1.Pod
		webserverType webserver
	}{
		{
			name:      "Clone Container not present Apache",
			Webserver: v1alpha1.Webserver{Image: "foo/bar:1"},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{},
					},
				},
			},
			expected: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: apacheAgentConfigVolume,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: apacheAgentVolume,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    apacheAgentCloneContainerName,
							Image:   "",
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{"cp -r /usr/local/apache2/conf/* " + webserverAgentDirectory + webserverAgentConfigDirectory},
							VolumeMounts: []corev1.VolumeMount{{
								Name:      apacheAgentConfigVolume,
								MountPath: webserverAgentDirectory + webserverAgentConfigDirectory,
							}},
						},
						{
							Name:    apacheAgentInitContainerName,
							Image:   "foo/bar:1",
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								"cp -ar /opt/opentelemetry/* /opt/opentelemetry-webserver/agent && export agentLogDir=$(echo \"/opt/opentelemetry-webserver/agent/logs\" | sed 's,/,\\\\/,g') && cat /opt/opentelemetry-webserver/agent/conf/appdynamics_sdk_log4cxx.xml.template | sed 's/__agent_log_dir__/'${agentLogDir}'/g'  > /opt/opentelemetry-webserver/agent/conf/appdynamics_sdk_log4cxx.xml && echo \"$OTEL_APACHE_AGENT_CONF\" > /opt/opentelemetry-webserver/source-conf/opentelemetry_module.conf && sed -i 's/<<SID-PLACEHOLDER>>/'${APACHE_SERVICE_INSTANCE_ID}'/g' /opt/opentelemetry-webserver/source-conf/opentelemetry_module.conf && echo 'Include /usr/local/apache2/conf/opentelemetry_module.conf' >> /opt/opentelemetry-webserver/source-conf/httpd.conf",
							},
							Env: []corev1.EnvVar{
								{
									Name:  apacheAttributesEnvVar,
									Value: "\n#Load the Otel Webserver SDK\nLoadFile /opt/opentelemetry-webserver/agent/sdk_lib/lib/libopentelemetry_common.so\nLoadFile /opt/opentelemetry-webserver/agent/sdk_lib/lib/libopentelemetry_resources.so\nLoadFile /opt/opentelemetry-webserver/agent/sdk_lib/lib/libopentelemetry_trace.so\nLoadFile /opt/opentelemetry-webserver/agent/sdk_lib/lib/libopentelemetry_otlp_recordable.so\nLoadFile /opt/opentelemetry-webserver/agent/sdk_lib/lib/libopentelemetry_exporter_ostream_span.so\nLoadFile /opt/opentelemetry-webserver/agent/sdk_lib/lib/libopentelemetry_exporter_otlp_grpc.so\n#Load the Otel ApacheModule SDK\nLoadFile /opt/opentelemetry-webserver/agent/sdk_lib/lib/libopentelemetry_webserver_sdk.so\n#Load the Apache Module. In this example for Apache 2.4\n#LoadModule otel_apache_module /opt/opentelemetry-webserver/agent/WebServerModule/Apache/libmod_apache_otel.so\n#Load the Apache Module. In this example for Apache 2.2\n#LoadModule otel_apache_module /opt/opentelemetry-webserver/agent/WebServerModule/Apache/libmod_apache_otel22.so\nLoadModule otel_apache_module /opt/opentelemetry-webserver/agent/WebServerModule/Apache/libmod_apache_otel.so\n#Attributes\nApacheModuleEnabled ON\nApacheModuleOtelExporterEndpoint http://otlp-endpoint:4317\nApacheModuleOtelSpanExporter otlp\nApacheModuleResolveBackends  ON\nApacheModuleServiceInstanceId <<SID-PLACEHOLDER>>\nApacheModuleServiceName apache-service-name\nApacheModuleServiceNamespace \nApacheModuleTraceAsError  ON\n",
								},
								{Name: apacheServiceInstanceIdEnvVar,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      apacheAgentVolume,
									MountPath: webserverAgentDirectory + webserverAgentSubDirectory,
								},
								{
									Name:      apacheAgentConfigVolume,
									MountPath: webserverAgentConfDirFull,
								},
								{
									Name:      apacheAgentConfigVolume,
									MountPath: apacheConfigDirectory,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      apacheAgentVolume,
									MountPath: webserverAgentDirectory + webserverAgentSubDirectory,
								},
								{
									Name:      apacheAgentConfigVolume,
									MountPath: apacheConfigDirectory,
								},
							},
						},
					},
				},
			},
			webserverType: apache,
		},
		{
			name:      "Clone Container not present Nginx",
			Webserver: v1alpha1.Webserver{Image: "foo/bar:1"},
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{},
					},
				},
			},
			expected: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: nginxAgentConfigVolume,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: nginxAgentVolume,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    nginxAgentCloneContainerName,
							Image:   "",
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{"cp -r /etc/nginx/* " + webserverAgentDirectory + webserverAgentConfigDirectory},
							VolumeMounts: []corev1.VolumeMount{{
								Name:      nginxAgentConfigVolume,
								MountPath: webserverAgentDirectory + webserverAgentConfigDirectory,
							}},
						},
						{
							Name:    nginxAgentInitContainerName,
							Image:   "foo/bar:1",
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								"cp -ar /opt/opentelemetry/* /opt/opentelemetry-webserver/agent && export agentLogDir=$(echo \"/opt/opentelemetry-webserver/agent/logs\" | sed 's,/,\\\\/,g') && cat /opt/opentelemetry-webserver/agent/conf/appdynamics_sdk_log4cxx.xml.template | sed 's/__agent_log_dir__/'${agentLogDir}'/g'  > /opt/opentelemetry-webserver/agent/conf/appdynamics_sdk_log4cxx.xml && echo \"$OTEL_NGINX_AGENT_CONF\" > /opt/opentelemetry-webserver/source-conf/opentelemetry_module.conf && sed -i 's/<<SID-PLACEHOLDER>>/'${NGINX_SERVICE_INSTANCE_ID}'/g' /opt/opentelemetry-webserver/source-conf/opentelemetry_module.conf && echo \"load_module /opt/opentelemetry-webserver/agent/WebServerModule/Nginx/ngx_http_opentelemetry_module.so; $(cat /etc/nginx/nginx.conf)\" > /etc/nginx/nginx.conf && mkdir -p /etc/nginx/conf.d && cp /etc/nginx/opentelemetry_module.conf /etc/nginx/conf.d",
							},
							Env: []corev1.EnvVar{
								{
									Name:  nginxAttributesEnvVar,
									Value: "NginxModuleEnabled ON;\nNginxModuleOtelExporterEndpoint http://otlp-endpoint:4317;\nNginxModuleOtelSpanExporter otlp;\nNginxModuleResolveBackends ON;\nNginxModuleServiceInstanceId <<SID-PLACEHOLDER>>;\nNginxModuleServiceName apache-service-name;\nNginxModuleServiceNamespace ;\nNginxModuleTraceAsError ON;\n",
								},
								{Name: nginxServiceInstanceIdEnvVar,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      nginxAgentVolume,
									MountPath: webserverAgentDirectory + webserverAgentSubDirectory,
								},
								{
									Name:      nginxAgentConfigVolume,
									MountPath: webserverAgentConfDirFull,
								},
								{
									Name:      nginxAgentConfigVolume,
									MountPath: nginxConfigDirectory,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  libLibraryPath,
									Value: webserverAgentDirFull + webserverAgentLibSubDirectory,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      nginxAgentVolume,
									MountPath: webserverAgentDirectory + webserverAgentSubDirectory,
								},
								{
									Name:      nginxAgentConfigVolume,
									MountPath: nginxConfigDirectory,
								},
							},
						},
					},
				},
			},
			webserverType: nginx,
		},
	}

	resourceMap := map[string]string{
		string(semconv.K8SDeploymentNameKey): "apache-service-name",
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := injectWebserverAgent(test.Webserver, test.pod, 0, "http://otlp-endpoint:4317", resourceMap, test.webserverType)
			assert.Equal(t, test.expected, pod)
		})
	}
}
