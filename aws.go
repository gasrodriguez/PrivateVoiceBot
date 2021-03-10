package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"strings"
)

type Instance struct {
	Id          string
	Name        string
	Description string
	State       string
}

func (o *Instance) ToString() string {
	return fmt.Sprintf("%s\t%-50s\t%s\n", o.Name, o.Description, o.State)
}

type AwsClient struct {
	Session   *session.Session
	Ec2Client *ec2.EC2
	Instances []Instance
}

func (o *AwsClient) State(user string) string {
	err := o.describeInstances()
	if err != nil {
		return formatErr(err)
	}
	userInstance := o.userInstance(user)
	if userInstance == nil {
		return fmt.Sprintf("User `%s` has no access to AWS instances", user)
	}
	text := "Current state of the AWS instances:\n"
	block := ""
	for _, instance := range awsClient.Instances {
		block += instance.ToString()
	}
	text += wrapCodeBlock(block)
	return text
}

func (o *AwsClient) StartInstance(user string) string {
	userInstance := o.userInstance(user)
	if userInstance == nil {
		return fmt.Sprintf("User `%s` has no access to AWS instances", user)
	}
	startInstancesOutput, err := o.Ec2Client.StartInstances(&ec2.StartInstancesInput{
		DryRun:      aws.Bool(false),
		InstanceIds: []*string{&userInstance.Id},
	})
	if err != nil {
		return formatErr(err)
	}
	userInstance.State = *startInstancesOutput.StartingInstances[0].CurrentState.Name
	text := "Starting instance:\n"
	text += wrapCodeBlock(userInstance.ToString())
	return text
}

func (o *AwsClient) HibernateInstance(user string) string {
	userInstance := o.userInstance(user)
	if userInstance == nil {
		return fmt.Sprintf("User `%s` has no access to AWS instances", user)
	}
	stopInstancesOutput, err := o.Ec2Client.StopInstances(&ec2.StopInstancesInput{
		DryRun:      aws.Bool(false),
		Force:       nil,
		Hibernate:   aws.Bool(true),
		InstanceIds: []*string{&userInstance.Id},
	})
	if err != nil {
		return formatErr(err)
	}
	userInstance.State = *stopInstancesOutput.StoppingInstances[0].CurrentState.Name
	text := "Hibernating instance:\n"
	text += wrapCodeBlock(userInstance.ToString())
	return text
}

func (o *AwsClient) StopInstance(user string) string {
	userInstance := o.userInstance(user)
	if userInstance == nil {
		return fmt.Sprintf("User `%s` has no access to AWS instances", user)
	}
	stopInstancesOutput, err := o.Ec2Client.StopInstances(&ec2.StopInstancesInput{
		DryRun:      aws.Bool(false),
		InstanceIds: []*string{&userInstance.Id},
	})
	if err != nil {
		return formatErr(err)
	}
	userInstance.State = *stopInstancesOutput.StoppingInstances[0].CurrentState.Name
	text := "Stopping instance:\n"
	text += wrapCodeBlock(userInstance.ToString())
	return text
}

func (o *AwsClient) userInstance(user string) *Instance {
	for _, instance := range o.Instances {
		if strings.Contains(instance.Description, user) {
			return &instance
		}
	}
	return nil
}

func (o *AwsClient) describeInstances() error {
	describeInstancesOutput, err := o.Ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{})
	if err != nil {
		return err
	}

	o.Instances = make([]Instance, 0)
	for _, reservation := range describeInstancesOutput.Reservations {
		for _, instance := range reservation.Instances {
			name := ""
			description := ""
			for _, tag := range instance.Tags {
				switch *tag.Key {
				case "Name":
					name = *tag.Value
					break
				case "Description":
					description = *tag.Value
					break
				}
			}
			o.Instances = append(o.Instances, Instance{
				Id:          *instance.InstanceId,
				Name:        name,
				Description: description,
				State:       *instance.State.Name,
			})
		}
	}
	return nil
}

func NewAwsClient(accessKeyId, secretAccessKey, region string) *AwsClient {
	thisSession := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKeyId, secretAccessKey, ""),
	}))
	client := &AwsClient{
		Session:   thisSession,
		Ec2Client: ec2.New(thisSession),
	}
	err := client.describeInstances()
	checkError(err)
	return client
}

const blockMarker = "```\n"

func wrapCodeBlock(text string) string {
	return blockMarker + text + blockMarker
}

func formatErr(err error) string {
	return wrapCodeBlock(err.Error())
}
