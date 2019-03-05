// +build !lambdabinary

package sparta

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"text/template"
	"time"

	spartaCF "github.com/mweagle/Sparta/aws/cloudformation"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	nodeColorService     = "#720502"
	nodeColorEventSource = "#BF2803"
	nodeColorLambda      = "#F35B05"
	nodeColorAPIGateway  = "#06B5F5"
	nodeNameAPIGateway   = "API Gateway"
)

type cytoscapeData struct {
	ID               string `json:"id"`
	Image            string `json:"image"`
	BackgroundColor  string `json:"backgroundColor,omitempty"`
	Source           string `json:"source,omitempty"`
	Target           string `json:"target,omitempty"`
	Label            string `json:"label,omitempty"`
	DegreeCentrality int    `json:"degreeCentrality"`
}
type cytoscapeNode struct {
	Data    cytoscapeData `json:"data"`
	Classes string        `json:"classes,omitempty"`
}
type templateResource struct {
	KeyName string
	Data    string
}

func cytoscapeNodeID(rawData interface{}) (string, error) {
	bytes, bytesErr := json.Marshal(rawData)
	if bytesErr != nil {
		return "", bytesErr
	}
	hash := sha1.New()
	_, writeErr := hash.Write(bytes)
	if writeErr != nil {
		return "", writeErr
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeNode(nodes *[]*cytoscapeNode,
	nodeName string,
	nodeColor string,
	nodeImage string,
	logger *logrus.Logger) error {
	nodeID, nodeErr := cytoscapeNodeID(nodeName)
	if nodeErr != nil {
		return errors.Wrapf(nodeErr,
			"Failed to create nodeID for entry: %s",
			nodeName)
	}
	appendNode := &cytoscapeNode{
		Data: cytoscapeData{
			ID:    nodeID,
			Label: strings.Trim(nodeName, "\""),
		},
	}
	if nodeImage != "" {
		resourceItem := templateResourceForKey(nodeImage, logger)
		if resourceItem != nil {
			appendNode.Data.Image = fmt.Sprintf("data:image/svg+xml;base64,%s",
				base64.StdEncoding.EncodeToString([]byte(resourceItem.Data)))
		}
	}
	*nodes = append(*nodes, appendNode)
	return nil
}

func writeLink(nodes *[]*cytoscapeNode,
	fromNode string,
	toNode string,
	label string) error {
	nodeSource, nodeSourceErr := cytoscapeNodeID(fromNode)
	if nodeSourceErr != nil {
		return errors.Wrapf(nodeSourceErr,
			"Failed to create nodeID for entry: %s",
			fromNode)
	}
	nodeTarget, nodeTargetErr := cytoscapeNodeID(toNode)
	if nodeTargetErr != nil {
		return errors.Wrapf(nodeSourceErr,
			"Failed to create nodeID for entry: %s",
			toNode)
	}

	*nodes = append(*nodes, &cytoscapeNode{
		Data: cytoscapeData{
			ID:     fmt.Sprintf("%d", rand.Uint64()),
			Source: nodeSource,
			Target: nodeTarget,
			Label:  label,
		},
	})
	return nil
}
func templateResourceForKey(resourceKeyName string, logger *logrus.Logger) *templateResource {
	var resource *templateResource
	resourcePath := fmt.Sprintf("/resources/describe/%s",
		strings.TrimLeft(resourceKeyName, "/"))
	data, dataErr := _escFSString(false, resourcePath)
	if dataErr == nil {
		keyParts := strings.Split(resourcePath, "/")
		keyName := keyParts[len(keyParts)-1]
		resource = &templateResource{
			KeyName: keyName,
			Data:    data,
		}
		logger.WithFields(logrus.Fields{
			"Path":    resourcePath,
			"KeyName": keyName,
		}).Debug("Embedded resource")

	} else {
		logger.WithFields(logrus.Fields{
			"Path": resourcePath,
		}).Warn("Failed to embed resource")
	}
	return resource
}
func templateResourcesForKeys(resourceKeyNames []string, logger *logrus.Logger) []*templateResource {
	var resources []*templateResource

	for _, eachKey := range resourceKeyNames {
		loadedResource := templateResourceForKey(eachKey, logger)
		if loadedResource != nil {
			resources = append(resources, loadedResource)
		}
	}
	return resources
}

func templateCSSFiles(logger *logrus.Logger) []*templateResource {
	cssFiles := []string{"bootstrap-4.0.0/dist/css/bootstrap.min.css",
		"highlight.js/styles/xcode.css",
	}
	return templateResourcesForKeys(cssFiles, logger)
}

func templateJSFiles(logger *logrus.Logger) []*templateResource {
	jsFiles := []string{"jquery/jquery-3.3.1.min.js",
		"popper/popper.min.js",
		"bootstrap-4.0.0/dist/js/bootstrap.min.js",
		"highlight.js/highlight.pack.js",
		"dagre-0.8.2/dagre-0.8.2/dist/dagre.js",
		"cytoscapejs/cytoscape.js",
		"cytoscape.js-dagre-2.2.2/cytoscape.js-dagre-2.2.2/cytoscape-dagre.js",
		"sparta.js",
	}
	return templateResourcesForKeys(jsFiles, logger)
}

func templateImageMap(logger *logrus.Logger) map[string]string {
	images := []string{"SpartaHelmet256.png",
		"AWS-Architecture-Icons/Dark-BG/Compute/AWS-Lambda_Lambda-Function_dark-bg.svg",
		"AWS-Architecture-Icons/Dark-BG/Management & Governance/AWS-CloudFormation_Stack_dark-bg.svg",
	}
	resources := templateResourcesForKeys(images, logger)
	imageMap := make(map[string]string)
	for _, eachResource := range resources {
		imageMap[eachResource.KeyName] = base64.StdEncoding.EncodeToString([]byte(eachResource.Data))
	}
	return imageMap
}

// TODO - this should really be smarter, including
// looking at the referred resource to understand it's
// type
func iconForAWSResource(rawEmitter interface{}) string {
	jsonBytes, jsonBytesErr := json.Marshal(rawEmitter)
	if jsonBytesErr != nil {
		jsonBytes = make([]byte, 0)
	}
	canonicalRaw := strings.ToLower(string(jsonBytes))
	iconMappings := map[string]string{
		"dynamodb":   "AWS-Architecture-Icons/Dark-BG/Database/Amazon-DynamoDB.svg",
		"sqs":        "AWS-Architecture-Icons/Dark-BG/Application Integration/Amazon-Simple-Queue-Service-SQS.svg",
		"sns":        "AWS-Architecture-Icons/Dark-BG/Application Integration/Amazon-Simple-Notification-Service-SNS.svg",
		"cloudwatch": "AWS-Architecture-Icons/Dark-BG/Management & Governance/Amazon-CloudWatch.svg",
		"kinesis":    "AWS-Architecture-Icons/Dark-BG/Analytics/Amazon-Kinesis.svg",
		"s3":         "AWS-Architecture-Icons/Dark-BG/Storage/Amazon-Simple-Storage-Service-S3.svg",
		"codecommit": "AWS-Architecture-Icons/Dark-BG/Developer Tools/AWS-CodeCommit.svg",
	}
	// Return it if we have it...
	for eachKey, eachPath := range iconMappings {
		if strings.Contains(canonicalRaw, eachKey) {
			return eachPath
		}
	}
	return "AWS-Architecture-Icons/Dark-BG/_Group Icons/AWS-Cloud_dark-bg.svg"
}

// Describe produces a graphical representation of a service's Lambda and data sources.  Typically
// automatically called as part of a compiled golang binary via the `describe` command
// line option.
func Describe(serviceName string,
	serviceDescription string,
	lambdaAWSInfos []*LambdaAWSInfo,
	api *API,
	s3Site *S3Site,
	s3BucketName string,
	buildTags string,
	linkFlags string,
	outputWriter io.Writer,
	workflowHooks *WorkflowHooks,
	logger *logrus.Logger) error {

	validationErr := validateSpartaPreconditions(lambdaAWSInfos, logger)
	if validationErr != nil {
		return validationErr
	}
	buildID, buildIDErr := provisionBuildID("none", logger)
	if buildIDErr != nil {
		buildID = fmt.Sprintf("%d", time.Now().Unix())
	}
	var cloudFormationTemplate bytes.Buffer
	err := Provision(true,
		serviceName,
		serviceDescription,
		lambdaAWSInfos,
		api,
		s3Site,
		s3BucketName,
		false,
		false,
		buildID,
		"",
		buildTags,
		linkFlags,
		&cloudFormationTemplate,
		workflowHooks,
		logger)
	if nil != err {
		return err
	}

	tmpl, err := template.New("description").Parse(_escFSMustString(false, "/resources/describe/template.html"))
	if err != nil {
		return errors.New(err.Error())
	}

	cytoscapeElements := make([]*cytoscapeNode, 0)
	// Instead of inline mermaid stuff, we're going to stuff raw
	// json through. We can also include AWS images in the icon
	// using base64/encoded:
	// Example: https://cytoscape.github.io/cytoscape.js-tutorial-demo/datasets/social.json
	// Use the "fancy" CSS:
	// https://github.com/cytoscape/cytoscape.js-tutorial-demo/blob/gh-pages/stylesheets/fancy.json
	// Which is dynamically updated at: https://cytoscape.github.io/cytoscape.js-tutorial-demo/

	// Setup the root object
	writeErr := writeNode(&cytoscapeElements,
		serviceName,
		nodeColorService,
		"AWS-Architecture-Icons/Dark-BG/Management & Governance/AWS-CloudFormation.svg",
		logger)
	if writeErr != nil {
		return writeErr
	}
	for _, eachLambda := range lambdaAWSInfos {
		// Other cytoscape nodes
		// Create the node...
		writeErr = writeNode(&cytoscapeElements,
			eachLambda.lambdaFunctionName(),
			nodeColorLambda,
			"AWS-Architecture-Icons/Dark-BG/Compute/AWS-Lambda.svg",
			logger)
		if writeErr != nil {
			return writeErr
		}
		writeErr = writeLink(&cytoscapeElements,
			eachLambda.lambdaFunctionName(),
			serviceName,
			"")
		if writeErr != nil {
			return writeErr
		}
		// Create permission & event mappings
		// functions declared in this
		for _, eachPermission := range eachLambda.Permissions {
			nodes, err := eachPermission.descriptionInfo()
			if nil != err {
				return err
			}

			for _, eachNode := range nodes {
				name := strings.TrimSpace(eachNode.Name)
				link := strings.TrimSpace(eachNode.Relation)
				// Style it to have the Amazon color
				nodeColor := eachNode.Color
				if nodeColor == "" {
					nodeColor = nodeColorEventSource
				}

				writeErr = writeNode(&cytoscapeElements,
					name,
					nodeColor,
					iconForAWSResource(eachNode.Name),
					logger)
				if writeErr != nil {
					return writeErr
				}
				writeErr = writeLink(&cytoscapeElements,
					name,
					eachLambda.lambdaFunctionName(),
					link)
				if writeErr != nil {
					return writeErr
				}
			}
		}
		for index, eachEventSourceMapping := range eachLambda.EventSourceMappings {
			dynamicArn := spartaCF.DynamicValueToStringExpr(eachEventSourceMapping.EventSourceArn)
			jsonBytes, jsonBytesErr := json.Marshal(dynamicArn)
			if jsonBytesErr != nil {
				jsonBytes = []byte(fmt.Sprintf("%s-EventSourceMapping[%d]",
					eachLambda.lambdaFunctionName(),
					index))
			}
			nodeName := string(jsonBytes)
			writeErr = writeNode(&cytoscapeElements,
				nodeName,
				nodeColorEventSource,
				iconForAWSResource(dynamicArn),
				logger)
			if writeErr != nil {
				return writeErr
			}
			writeErr = writeLink(&cytoscapeElements,
				nodeName,
				eachLambda.lambdaFunctionName(),
				"")
			if writeErr != nil {
				return writeErr
			}
		}
	}

	// API?
	if nil != api {
		// Create the APIGateway virtual node && connect it to the application
		writeErr = writeNode(&cytoscapeElements,
			nodeNameAPIGateway,
			nodeColorAPIGateway,
			"AWS-Architecture-Icons/Dark-BG/Mobile/Amazon-API-Gateway.svg",
			logger)
		if writeErr != nil {
			return writeErr
		}
		for _, eachResource := range api.resources {
			for eachMethod := range eachResource.Methods {
				// Create the PATH node
				var nodeName = fmt.Sprintf("%s - %s", eachMethod, eachResource.pathPart)
				writeErr = writeNode(&cytoscapeElements,
					nodeName,
					nodeColorAPIGateway,
					"AWS-Architecture-Icons/Dark-BG/_Group Icons/AWS-Cloud_dark-bg.svg",
					logger)
				if writeErr != nil {
					return writeErr
				}
				writeErr = writeLink(&cytoscapeElements,
					nodeNameAPIGateway,
					nodeName,
					"")
				if writeErr != nil {
					return writeErr
				}
				writeErr = writeLink(&cytoscapeElements,
					nodeName,
					eachResource.parentLambda.lambdaFunctionName(),
					"")
				if writeErr != nil {
					return writeErr
				}
			}
		}
	}
	cytoscapeBytes, cytoscapeBytesErr := json.MarshalIndent(cytoscapeElements, "", " ")
	if cytoscapeBytesErr != nil {
		return errors.Wrapf(cytoscapeBytesErr, "Failed to marshal cytoscape data")
	}
	params := struct {
		SpartaVersion          string
		ServiceName            string
		ServiceDescription     string
		CloudFormationTemplate string
		CSSFiles               []*templateResource
		JSFiles                []*templateResource
		ImageMap               map[string]string
		CytoscapeData          interface{}
	}{
		SpartaGitHash[0:8],
		serviceName,
		serviceDescription,
		cloudFormationTemplate.String(),
		templateCSSFiles(logger),
		templateJSFiles(logger),
		templateImageMap(logger),
		string(cytoscapeBytes),
	}
	return tmpl.Execute(outputWriter, params)
}
