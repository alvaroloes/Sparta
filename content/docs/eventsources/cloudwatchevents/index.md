+++
author = "Matt Weagle"
date = "2015-11-29T06:50:17"
title = "Event Source - CloudWatch Events"
tags = ["sparta", "event_source"]
type = "doc"
+++

In this section we'll walkthrough how to trigger your lambda function in response to different types of [CloudWatch Events](https://aws.amazon.com/blogs/aws/new-cloudwatch-events-track-and-respond-to-changes-to-your-aws-resources/).  This overview is based on the [SpartaApplication](https://github.com/mweagle/SpartaApplication) sample code if you'd rather jump to the end result.

## Goal {#goal}  

Assume that we're supposed to write a simple "HelloWorld" CloudWatch event function that has two responsibilities:

  * Run every *5 minutes* to provide a heartbeat notification to our alerting system via a logfile entry
  * Log *EC2-related* events for later processing

## Getting Started {#gettingStarted}  

Our lambda function is pretty simple:

{{< highlight go >}}
func echoCloudWatchEvent(event *json.RawMessage,
                        context *sparta.LambdaContext,
                        w http.ResponseWriter,
                        logger *logrus.Logger) {

	logger.WithFields(logrus.Fields{
		"RequestID": context.AWSRequestID,
	}).Info("Request received")

	config, _ := sparta.Discover()
	logger.WithFields(logrus.Fields{
		"RequestID":     context.AWSRequestID,
		"Event":         string(*event),
		"Configuration": config,
	}).Info("Request received")
	fmt.Fprintf(w, "Hello World!")
}
{{< /highlight >}}   

Our lambda function doesn't need to do much with the event other than log it.

## Sparta Integration {#spartaIntegration}  

With the `echoCloudWatchEvent` implemented, the next step is to integrate the **Go** function with Sparta.  This is done by the `appendCloudWatchEventHandler` in the SpartaApplication [application.go](https://github.com/mweagle/SpartaApplication/blob/master/application.go) source.

Our lambda function only needs logfile write privileges, and since these are enabled by default, we can use an empty `sparta.IAMRoleDefinition` value:

{{< highlight go >}}
func appendCloudWatchEventHandler(api *sparta.API,
                                  lambdaFunctions []*sparta.LambdaAWSInfo) []*sparta.LambdaAWSInfo {

  lambdaFn := sparta.NewLambda(sparta.IAMRoleDefinition{}, echoCloudWatchEvent, nil)
{{< /highlight >}}   


The next step is to add a `CloudWatchEventsPermission` value that includes the two rule triggers.

{{< highlight go >}}
  cloudWatchEventsPermission := sparta.CloudWatchEventsPermission{}
cloudWatchEventsPermission.Rules = make(map[string]sparta.CloudWatchEventsRule, 0)
{{< /highlight >}}   

Our two rules will be inserted into the `Rules` map in the next steps.

### Cron Expression {#cronExpression}

Our first requirement is that the lambda function write a heartbeat to the logfile every 5 mins.  This can be configured by adding a scheduled event:

{{< highlight go >}}
cloudWatchEventsPermission.Rules["Rate5Mins"] = sparta.CloudWatchEventsRule{
  ScheduleExpression: "rate(5 minutes)",
}
{{< /highlight >}}  

The `ScheduleExpression` value can either be a _rate_ or a _cron_ [expression](http://docs.aws.amazon.com/AmazonCloudWatch/latest/DeveloperGuide/ScheduledEvents.html).  The map keyname is used when adding the [rule](http://docs.aws.amazon.com/AWSJavaScriptSDK/latest/AWS/CloudWatchEvents.html#putRule-property) during stack provisioning.

### Event Pattern {#eventPattern}

The other requirement is that our lambda function be notified when matching EC2 events are created.  To support this, we'll add a second `Rule`:

{{< highlight go >}}
cloudWatchEventsPermission.Rules["EC2Activity"] = sparta.CloudWatchEventsRule{
  EventPattern: map[string]interface{}{
    "source":      []string{"aws.ec2"},
    "detail-type": []string{"EC2 Instance State-change Notification"},
  },
}
{{< /highlight >}}  

The EC2 event pattern is the **Go** JSON-compatible representation of the event pattern that CloudWatch Events will use to trigger our lambda function.  This structured value will be marshaled to a String during CloudFormation Template marshaling.

<div class="panel panel-warning">
  <div class="panel-heading">Validity Checks</div>
   <div class="panel-body">
    Sparta does <b>NOT</b> attempt to validate either <code>ScheduleExpression</code> or <code>EventPattern</code> values prior to calling CloudFormation.  Syntax errors in either value will be detected during provisioning when the Sparta CloudFormation CustomResource calls <a href="http://docs.aws.amazon.com/AWSJavaScriptSDK/latest/AWS/CloudWatchEvents.html#putRule-property">putRule</a> to add the lambda target.  This error will cause the CloudFormation operation to fail.  Any API errors will be logged & are viewable in the <a href="https://blogs.aws.amazon.com/application-management/post/TxPYD8JT4CB5UY/View-CloudFormation-Logs-in-the-Console">CloudFormation Logs Console</a>.
   </div>
</div>

### Add Permission {#addPermission}

With the two rules configured, the final step is to add the `sparta.CloudWatchPermission` to our `sparta.LambdaAWSInfo` value:

{{< highlight go >}}
lambdaFn.Permissions = append(lambdaFn.Permissions, cloudWatchEventsPermission)
return append(lambdaFunctions, lambdaFn)
{{< /highlight >}}  

Our entire function is therefore:

{{< highlight go >}}
func appendCloudWatchEventHandler(api *sparta.API,
                                  lambdaFunctions []*sparta.LambdaAWSInfo) []*sparta.LambdaAWSInfo {

  lambdaFn := sparta.NewLambda(sparta.IAMRoleDefinition{}, echoCloudWatchEvent, nil)

  cloudWatchEventsPermission := sparta.CloudWatchEventsPermission{}
  cloudWatchEventsPermission.Rules = make(map[string]sparta.CloudWatchEventsRule, 0)
  cloudWatchEventsPermission.Rules["Rate5Mins"] = sparta.CloudWatchEventsRule{
    ScheduleExpression: "rate(5 minutes)",
  }
  cloudWatchEventsPermission.Rules["EC2Activity"] = sparta.CloudWatchEventsRule{
    EventPattern: map[string]interface{}{
    	"source":      []string{"aws.ec2"},
    	"detail-type": []string{"EC2 Instance State-change Notification"},
    },
  }
  lambdaFn.Permissions = append(lambdaFn.Permissions, cloudWatchEventsPermission)

  return append(lambdaFunctions, lambdaFn)
}
{{< /highlight >}}  


## <a href="{{< relref "#wrappingUp" >}}">Wrapping Up</a>

With the `lambdaFn` fully defined, we can provide it to `sparta.Main()` and deploy our service.  The workflow below is shared by all CloudWatch Events-triggered lambda functions:

  * Define the lambda function (`echoCloudWatchEvent`).
  * If needed, create the required [IAMRoleDefinition](https://godoc.org/github.com/mweagle/Sparta*IAMRoleDefinition) with appropriate privileges.
  * Provide the lambda function & IAMRoleDefinition to `sparta.NewLambda()`
  * Create a [CloudWatchEventsPermission](https://godoc.org/github.com/mweagle/Sparta#CloudWatchEventsPermission) value.
  * Add one or more [CloudWatchEventsRules](https://godoc.org/github.com/mweagle/Sparta#CloudWatchEventsRule) to the `CloudWatchEventsPermission.Rules` map that define your lambda function's trigger condition:
    * [Scheduled Events](http://docs.aws.amazon.com/AmazonCloudWatch/latest/DeveloperGuide/ScheduledEvents.html)
    * [Event Patterns](http://docs.aws.amazon.com/AmazonCloudWatch/latest/DeveloperGuide/CloudWatchEventsandEventPatterns.html)
  * Append the `CloudWatchEventsPermission` value to the lambda function's `Permissions` slice.
  * Include the reference in the call to `sparta.Main()`.

## <a href="{{< relref "#otherResources" >}}">Other Resources</a>

  * Introduction to [CloudWatch Events](https://aws.amazon.com/blogs/aws/new-cloudwatch-events-track-and-respond-to-changes-to-your-aws-resources/)
  * [Run an AWS Lambda Function on a Schedule Using the AWS CLI](http://docs.aws.amazon.com/AmazonCloudWatch/latest/DeveloperGuide/RunLambdaSchedule.html)
  * The EC2 event pattern is drawn from the AWS [Events & Event Patterns](http://docs.aws.amazon.com/AmazonCloudWatch/latest/DeveloperGuide/CloudWatchEventsandEventPatterns.html) documentation