package main

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/artifactregistry"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/cloudrunv2"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/firebase"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/organizations"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/secretmanager"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/sql"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/storage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(run)
}

func run(ctx *pulumi.Context) error {
	cfg := config.New(ctx, "")
	gcpCfg := config.New(ctx, "gcp")

	project := gcpCfg.Require("project")
	region := gcpCfg.Get("region")
	if region == "" {
		region = "us-central1"
	}

	sqlTier := cfg.Get("sqlTier")
	if sqlTier == "" {
		sqlTier = "db-f1-micro"
	}

	minInstances, err := cfg.TryInt("minInstances")
	if err != nil {
		minInstances = 0
	}

	sqlBackups, err := cfg.TryBool("sqlBackups")
	if err != nil {
		sqlBackups = false
	}

	enableSecrets, err := cfg.TryBool("enableSecrets")
	if err != nil {
		enableSecrets = false
	}

	apiTag := cfg.Get("apiTag")
	if apiTag == "" {
		apiTag = "latest"
	}
	vssTag := cfg.Get("vssTag")
	if vssTag == "" {
		vssTag = "latest"
	}
	settlementTag := cfg.Get("settlementTag")
	if settlementTag == "" {
		settlementTag = "latest"
	}
	registryBase := fmt.Sprintf("%s-docker.pkg.dev/%s/apex-check-deposit", region, project)

	// ── Artifact Registry ────────────────────────────────────────────────────
	_, err = artifactregistry.NewRepository(ctx, "apex-repo", &artifactregistry.RepositoryArgs{
		Project:      pulumi.String(project),
		Location:     pulumi.String(region),
		RepositoryId: pulumi.String("apex-check-deposit"),
		Format:       pulumi.String("DOCKER"),
	})
	if err != nil {
		return err
	}

	// ── Cloud SQL (Postgres 16) ───────────────────────────────────────────────
	sqlInstance, err := sql.NewDatabaseInstance(ctx, "apex-postgres", &sql.DatabaseInstanceArgs{
		Project:         pulumi.String(project),
		Region:          pulumi.String(region),
		DatabaseVersion: pulumi.String("POSTGRES_16"),
		Settings: &sql.DatabaseInstanceSettingsArgs{
			Tier: pulumi.String(sqlTier),
			BackupConfiguration: &sql.DatabaseInstanceSettingsBackupConfigurationArgs{
				Enabled: pulumi.Bool(sqlBackups),
			},
			IpConfiguration: &sql.DatabaseInstanceSettingsIpConfigurationArgs{
				Ipv4Enabled: pulumi.Bool(true),
			},
		},
		DeletionProtection: pulumi.Bool(false),
	})
	if err != nil {
		return err
	}

	_, err = sql.NewDatabase(ctx, "apex-db", &sql.DatabaseArgs{
		Project:  pulumi.String(project),
		Instance: sqlInstance.Name,
		Name:     pulumi.String("apex_check_deposit"),
	})
	if err != nil {
		return err
	}

	_, err = sql.NewUser(ctx, "apex-user", &sql.UserArgs{
		Project:  pulumi.String(project),
		Instance: sqlInstance.Name,
		Name:     pulumi.String("apex"),
		Password: pulumi.String("apex"),
	})
	if err != nil {
		return err
	}

	// Database URL via Cloud SQL Unix socket (Cloud Run built-in connector)
	dbURL := sqlInstance.ConnectionName.ApplyT(func(connName string) string {
		return fmt.Sprintf(
			"postgresql://apex:apex@/apex_check_deposit?host=/cloudsql/%s&sslmode=disable",
			connName,
		)
	}).(pulumi.StringOutput)

	// ── GCS bucket for check images ──────────────────────────────────────────
	_, err = storage.NewBucket(ctx, "apex-deposits", &storage.BucketArgs{
		Project:                  pulumi.String(project),
		Location:                 pulumi.String(region),
		Name:                     pulumi.Sprintf("apex-deposits-%s", project),
		UniformBucketLevelAccess: pulumi.Bool(true),
	})
	if err != nil {
		return err
	}

	// ── Secret Manager (prod only) ───────────────────────────────────────────
	if enableSecrets {
		jwtSecret, err := secretmanager.NewSecret(ctx, "jwt-secret", &secretmanager.SecretArgs{
			Project:  pulumi.String(project),
			SecretId: pulumi.String("JWT_SECRET"),
			Replication: &secretmanager.SecretReplicationArgs{
				Auto: &secretmanager.SecretReplicationAutoArgs{},
			},
		})
		if err != nil {
			return err
		}
		_, err = secretmanager.NewSecretVersion(ctx, "jwt-secret-v1", &secretmanager.SecretVersionArgs{
			Secret:     jwtSecret.ID(),
			SecretData: pulumi.String("changeme-before-first-deploy"),
		})
		if err != nil {
			return err
		}

		settlementSecret, err := secretmanager.NewSecret(ctx, "settlement-token", &secretmanager.SecretArgs{
			Project:  pulumi.String(project),
			SecretId: pulumi.String("SETTLEMENT_BANK_TOKEN"),
			Replication: &secretmanager.SecretReplicationArgs{
				Auto: &secretmanager.SecretReplicationAutoArgs{},
			},
		})
		if err != nil {
			return err
		}
		_, err = secretmanager.NewSecretVersion(ctx, "settlement-token-v1", &secretmanager.SecretVersionArgs{
			Secret:     settlementSecret.ID(),
			SecretData: pulumi.String("changeme-before-first-deploy"),
		})
		if err != nil {
			return err
		}

		ctx.Export("jwtSecretName", jwtSecret.Name)
		ctx.Export("settlementTokenSecretName", settlementSecret.Name)
	}

	// ── Cloud Run: VSS stub ──────────────────────────────────────────────────
	// Deploy VSS first — no external dependencies
	vssService, err := cloudrunv2.NewService(ctx, "vss", &cloudrunv2.ServiceArgs{
		Project:  pulumi.String(project),
		Location: pulumi.String(region),
		Ingress:  pulumi.String("INGRESS_TRAFFIC_ALL"),
		Template: &cloudrunv2.ServiceTemplateArgs{
			Scaling: &cloudrunv2.ServiceTemplateScalingArgs{
				MinInstanceCount: pulumi.Int(minInstances),
				MaxInstanceCount: pulumi.Int(5),
			},
			Containers: cloudrunv2.ServiceTemplateContainerArray{
				&cloudrunv2.ServiceTemplateContainerArgs{
					Image: pulumi.String(fmt.Sprintf("%s/vss:%s", registryBase, vssTag)),
					Ports: cloudrunv2.ServiceTemplateContainerPortArray{
						&cloudrunv2.ServiceTemplateContainerPortArgs{
							ContainerPort: pulumi.Int(8081),
						},
					},
					Envs: cloudrunv2.ServiceTemplateContainerEnvArray{
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("VSS_PORT"),
							Value: pulumi.String("8081"),
						},
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("SCENARIOS_PATH"),
							Value: pulumi.String("/scenarios/scenarios.yaml"),
						},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	// NOTE: IAM bindings removed — org policy blocks allUsers/allAuthenticatedUsers/domain.
	// Use gcloud to grant access manually after deploy.

	// ── Cloud Run: API ───────────────────────────────────────────────────────
	// API depends on VSS URI and Cloud SQL connection name
	apiService, err := cloudrunv2.NewService(ctx, "api", &cloudrunv2.ServiceArgs{
		Project:  pulumi.String(project),
		Location: pulumi.String(region),
		Ingress:  pulumi.String("INGRESS_TRAFFIC_ALL"),
		Template: &cloudrunv2.ServiceTemplateArgs{
			Scaling: &cloudrunv2.ServiceTemplateScalingArgs{
				MinInstanceCount: pulumi.Int(minInstances),
				MaxInstanceCount: pulumi.Int(10),
			},
			Containers: cloudrunv2.ServiceTemplateContainerArray{
				&cloudrunv2.ServiceTemplateContainerArgs{
					Image: pulumi.String(fmt.Sprintf("%s/api:%s", registryBase, apiTag)),
					Ports: cloudrunv2.ServiceTemplateContainerPortArray{
						&cloudrunv2.ServiceTemplateContainerPortArgs{
							ContainerPort: pulumi.Int(8080),
						},
					},
					Envs: cloudrunv2.ServiceTemplateContainerEnvArray{
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("API_PORT"),
							Value: pulumi.String("8080"),
						},
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("DATABASE_URL"),
							Value: dbURL,
						},
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("VSS_URL"),
							Value: vssService.Uri,
						},
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("JWT_SECRET"),
							Value: pulumi.String("dev-secret-change-in-production"),
						},
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("SETTLEMENT_BANK_TOKEN"),
							Value: pulumi.String("dev-settlement-token"),
						},
					},
					VolumeMounts: cloudrunv2.ServiceTemplateContainerVolumeMountArray{
						&cloudrunv2.ServiceTemplateContainerVolumeMountArgs{
							Name:      pulumi.String("cloudsql"),
							MountPath: pulumi.String("/cloudsql"),
						},
					},
					StartupProbe: &cloudrunv2.ServiceTemplateContainerStartupProbeArgs{
						TcpSocket: &cloudrunv2.ServiceTemplateContainerStartupProbeTcpSocketArgs{
							Port: pulumi.Int(8080),
						},
						InitialDelaySeconds: pulumi.Int(5),
						PeriodSeconds:       pulumi.Int(10),
						FailureThreshold:    pulumi.Int(20),
						TimeoutSeconds:      pulumi.Int(5),
					},
				},
			},
			Volumes: cloudrunv2.ServiceTemplateVolumeArray{
				&cloudrunv2.ServiceTemplateVolumeArgs{
					Name: pulumi.String("cloudsql"),
					CloudSqlInstance: &cloudrunv2.ServiceTemplateVolumeCloudSqlInstanceArgs{
						Instances: pulumi.StringArray{sqlInstance.ConnectionName},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	// ── Cloud Run: Settlement stub ───────────────────────────────────────────
	// Settlement depends on API URI for return webhooks
	settlementService, err := cloudrunv2.NewService(ctx, "settlement", &cloudrunv2.ServiceArgs{
		Project:  pulumi.String(project),
		Location: pulumi.String(region),
		Ingress:  pulumi.String("INGRESS_TRAFFIC_ALL"),
		Template: &cloudrunv2.ServiceTemplateArgs{
			Scaling: &cloudrunv2.ServiceTemplateScalingArgs{
				MinInstanceCount: pulumi.Int(minInstances),
				MaxInstanceCount: pulumi.Int(5),
			},
			Containers: cloudrunv2.ServiceTemplateContainerArray{
				&cloudrunv2.ServiceTemplateContainerArgs{
					Image: pulumi.String(fmt.Sprintf("%s/settlement:%s", registryBase, settlementTag)),
					Ports: cloudrunv2.ServiceTemplateContainerPortArray{
						&cloudrunv2.ServiceTemplateContainerPortArgs{
							ContainerPort: pulumi.Int(8082),
						},
					},
					Envs: cloudrunv2.ServiceTemplateContainerEnvArray{
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("SETTLEMENT_PORT"),
							Value: pulumi.String("8082"),
						},
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("API_URL"),
							Value: apiService.Uri,
						},
						&cloudrunv2.ServiceTemplateContainerEnvArgs{
							Name:  pulumi.String("SETTLEMENT_BANK_TOKEN"),
							Value: pulumi.String("dev-settlement-token"),
						},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	// ── Firebase Hosting (replaces Cloud Run web frontend) ──────────────────
	hostingSite, err := firebase.NewHostingSite(ctx, "apex-web", &firebase.HostingSiteArgs{
		Project: pulumi.String(project),
		SiteId:  pulumi.Sprintf("apex-check-deposit-%s", ctx.Stack()),
	})
	if err != nil {
		return err
	}

	// Look up project number for service account references
	projectInfo, err := organizations.LookupProject(ctx, &organizations.LookupProjectArgs{
		ProjectId: &project,
	})
	if err != nil {
		return err
	}
	projectNumber := projectInfo.Number

	// Grant default compute SA permission to invoke Cloud Run services
	// (Firebase Hosting rewrites use this SA to proxy requests)
	for _, svc := range []struct {
		name    string
		service pulumi.StringInput
	}{
		{"api", apiService.Name},
		{"vss", vssService.Name},
		{"settlement", settlementService.Name},
	} {
		_, err = cloudrunv2.NewServiceIamMember(ctx, fmt.Sprintf("%s-compute-invoker", svc.name), &cloudrunv2.ServiceIamMemberArgs{
			Project:  pulumi.String(project),
			Location: pulumi.String(region),
			Name:     svc.service,
			Role:     pulumi.String("roles/run.invoker"),
			Member:   pulumi.Sprintf("serviceAccount:%s-compute@developer.gserviceaccount.com", projectNumber),
		})
		if err != nil {
			return err
		}

		_, err = cloudrunv2.NewServiceIamMember(ctx, fmt.Sprintf("%s-firebase-invoker", svc.name), &cloudrunv2.ServiceIamMemberArgs{
			Project:  pulumi.String(project),
			Location: pulumi.String(region),
			Name:     svc.service,
			Role:     pulumi.String("roles/run.invoker"),
			Member:   pulumi.Sprintf("serviceAccount:service-%s@gcp-sa-firebase.iam.gserviceaccount.com", projectNumber),
		})
		if err != nil {
			return err
		}
	}

	// ── Outputs ──────────────────────────────────────────────────────────────
	ctx.Export("apiUrl", apiService.Uri)
	ctx.Export("apiServiceName", apiService.Name)
	ctx.Export("vssUrl", vssService.Uri)
	ctx.Export("settlementUrl", settlementService.Uri)
	ctx.Export("webUrl", hostingSite.DefaultUrl)
	ctx.Export("dbConnectionName", sqlInstance.ConnectionName)

	return nil
}
