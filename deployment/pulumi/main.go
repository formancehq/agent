package main

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		k8s, err := newK8sSetup(ctx, cfg)
		if err != nil {
			return err
		}
		namespace := k8s.Namespace
		k8sProvider := k8s.Provider

		dc := newDockerConfig(ctx, cfg)

		// Build agent image
		agentImage, err := dc.buildImage(ctx, "formancehq/agent", "../..", "../../Dockerfile")
		if err != nil {
			return fmt.Errorf("failed to build agent image: %w", err)
		}

		// Agent configuration
		serverAddress := cfg.Get("server-address")
		if serverAddress == "" {
			serverAddress = "app.formance.cloud:443"
		}

		tlsEnabled := true
		if v := cfg.Get("tls-enabled"); v == "false" {
			tlsEnabled = false
		}

		tlsInsecureSkipVerify := true
		if v := cfg.Get("tls-insecure-skip-verify"); v == "false" {
			tlsInsecureSkipVerify = false
		}

		agentID := cfg.Require("agent-id")
		baseURL := cfg.Require("base-url")

		authMode := cfg.Get("authentication-mode")
		if authMode == "" {
			authMode = "bearer"
		}

		authIssuer := cfg.Get("authentication-issuer")
		if authIssuer == "" {
			authIssuer = "https://app.formance.cloud/api"
		}

		production := cfg.GetBool("production")
		outdated := cfg.GetBool("outdated")
		debug := cfg.GetBool("debug")

		// Build Helm values
		helmValues := pulumi.Map{
			"global": pulumi.Map{
				"debug": pulumi.Bool(debug),
			},
			"image": pulumi.Map{
				"repository": pulumi.Sprintf("%s/formancehq/agent", dc.PullRegistry),
				"tag":        pulumi.Sprintf("latest@%s", agentImage.Digest),
			},
			"server": pulumi.Map{
				"address": pulumi.String(serverAddress),
				"tls": pulumi.Map{
					"enabled":            pulumi.Bool(tlsEnabled),
					"insecureSkipVerify": pulumi.Bool(tlsInsecureSkipVerify),
				},
			},
			"agent": pulumi.Map{
				"id":         pulumi.String(agentID),
				"baseUrl":    pulumi.String(baseURL),
				"production": pulumi.Bool(production),
				"outdated":   pulumi.Bool(outdated),
				"authentication": agentAuthValues(ctx, authMode, authIssuer, agentID),
			},
			"imagePullSecrets": getImagePullSecrets(cfg),
			"nodeSelector":     getConfigMap(cfg, "node-selector"),
			"tolerations":      getConfigArray(cfg, "tolerations"),
		}

		// Additional base URLs
		additionalBaseURLs := cfg.Get("additional-base-urls")
		if additionalBaseURLs != "" {
			urls := strings.Split(additionalBaseURLs, ",")
			arr := make(pulumi.Array, len(urls))
			for i, u := range urls {
				arr[i] = pulumi.String(strings.TrimSpace(u))
			}
			helmValues["agent"].(pulumi.Map)["additionalBaseUrls"] = arr
		}

		// Deploy agent via Helm chart from formancehq/helm repo
		agentRelease, err := helm.NewRelease(ctx, "agent", &helm.ReleaseArgs{
			Name:  pulumi.String("agent"),
			Chart: pulumi.String("oci://ghcr.io/formancehq/helm/agent"),
			// Alternatively, use a local checkout:
			// Chart: pulumi.String("../../path-to-helm-charts/charts/agent"),
			Namespace:   pulumi.String(namespace),
			Values:      helmValues,
			ForceUpdate: pulumi.Bool(true),
		},
			pulumi.DependsOn([]pulumi.Resource{agentImage.Resource()}),
			pulumi.Provider(k8sProvider),
		)
		if err != nil {
			return fmt.Errorf("failed to deploy agent: %w", err)
		}

		// Exports
		ctx.Export("namespace", pulumi.String(namespace))
		ctx.Export("agentImage", pulumi.Sprintf("%s/formancehq/agent:latest@%s", dc.PullRegistry, agentImage.Digest))
		ctx.Export("agentRelease", agentRelease.Name)

		return nil
	})
}

func agentAuthValues(ctx *pulumi.Context, mode, issuer, agentID string) pulumi.Map {
	auth := pulumi.Map{
		"mode": pulumi.String(mode),
	}

	switch mode {
	case "token":
		auth["token"] = config.GetSecret(ctx, "authentication-client-secret")
	default: // bearer
		auth["issuer"] = pulumi.String(issuer)
		auth["clientID"] = pulumi.String(agentID)
		auth["clientSecret"] = config.GetSecret(ctx, "authentication-client-secret")
	}

	return auth
}
