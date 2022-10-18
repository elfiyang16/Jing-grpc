package internal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/manifoldco/promptui"
)

type portMapping struct {
	taskARN       string
	ec2InstanceID string
	host          int32
	container     int32
}

func (p portMapping) String() string {
	return fmt.Sprintf("{ Task: %s, EC2 Instance: %s, Host Port: %d, Container Port: %d }", p.taskARN, p.ec2InstanceID, p.host, p.container)
}

type portalGun struct {
	ecsClient *ecs.Client
}

func NewPortalGun() portalGun {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		fmt.Println(err)
	}
	ecsClient := ecs.NewFromConfig(cfg)
	return portalGun{ecsClient: ecsClient}
}

// Portal opens a portal from your local port to whichever port you select
func (pg *portalGun) Portal(ctx context.Context, hopperApp string, hopperService string, port int) (<-chan string, error) {
	tasks, err := pg.getTasksForService(ctx, "staging", hopperApp, hopperService)
	if err != nil {
		return nil, err
	}

	portMappings, err := pg.getPortMappings(ctx, tasks, "staging")
	prompt := promptui.Select{
		Label: "Select the task to port forward to",
		Items: portMappings,
	}
	i, _, err := prompt.Run()
	if err != nil {
		return nil, err
	}

	msgStream, err := startSSMPortForward(ctx, port, portMappings[i])
	if err != nil {
		return nil, err
	}
	return msgStream, nil
}

// getTasksForService gets all the tasks associated with the given service name in the given cluster
// Currently only the staging cluster is supported.
func (pg *portalGun) getTasksForService(ctx context.Context, cluster string, app string, service string) ([]types.Task, error) {
	serviceName := fmt.Sprintf("%s-%s", app, service)
	listTasksOutput, err := pg.ecsClient.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster:       &cluster,
		DesiredStatus: types.DesiredStatusRunning,
		ServiceName:   &serviceName,
	})
	if err != nil {
		return nil, err
	}
	if len(listTasksOutput.TaskArns) < 1 {
		return nil, errors.New(fmt.Sprintf("could not find any tasks in cluster %s with service name %s", cluster, serviceName))
	}
	taskarns := listTasksOutput.TaskArns

	tasks := make([]types.Task, len(taskarns))
	for i, arn := range taskarns {
		describeTasksOutput, err := pg.ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Tasks:   []string{arn},
			Cluster: &cluster,
		})
		if err != nil {
			return nil, err
		}
		if len(describeTasksOutput.Tasks) < 1 {
			return nil, errors.New(fmt.Sprintf("found 0 tasks with arn: %s", arn))
		}
		if len(describeTasksOutput.Tasks) > 1 {
			return nil, errors.New(fmt.Sprintf("found multiple tasks with arn: %s", arn))
		}
		tasks[i] = describeTasksOutput.Tasks[0]
	}
	return tasks, nil
}

// getPortMappings joins a task with the EC2 instance running it and the containers running on that EC2 instance.
func (pg *portalGun) getPortMappings(ctx context.Context, tasks []types.Task, cluster string) ([]portMapping, error) {
	portMappings := make([]portMapping, 0)
	for _, task := range tasks {
		ec2InstanceID, err := pg.getEC2InstanceIDForTask(ctx, task, cluster)
		if err != nil {
			return nil, err
		}
		for _, container := range task.Containers {
			for _, networkBinding := range container.NetworkBindings {
				portMappings = append(portMappings, portMapping{
					taskARN:       *task.TaskArn,
					ec2InstanceID: ec2InstanceID,
					host:          *networkBinding.HostPort,
					container:     *networkBinding.ContainerPort,
				})
			}
		}
	}
	return portMappings, nil
}

func (pg *portalGun) getEC2InstanceIDForTask(ctx context.Context, task types.Task, cluster string) (string, error) {
	describeContainerInstancesOutput, err := pg.ecsClient.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
		ContainerInstances: []string{*task.ContainerInstanceArn},
		Cluster:            &cluster,
	})
	if err != nil {
		return "", err
	}
	if len(describeContainerInstancesOutput.ContainerInstances) < 1 {
		return "", errors.New(fmt.Sprintf("found 0 container instances for task: %s", *task.TaskArn))
	}
	if len(describeContainerInstancesOutput.ContainerInstances) > 1 {
		return "", errors.New(fmt.Sprintf("found multiple container instances for task: %s", *task.TaskArn))
	}

	ec2InstanceID := describeContainerInstancesOutput.ContainerInstances[0].Ec2InstanceId
	if ec2InstanceID == nil {
		return "", errors.New(fmt.Sprintf(
			"container instance %s had a nil EC2 instance ID",
			*describeContainerInstancesOutput.ContainerInstances[0].ContainerInstanceArn,
		))
	}
	return *ec2InstanceID, nil
}

// startSSMPortForward starts a port forwarding session from localPort to the selected EC2 instance.
func startSSMPortForward(ctx context.Context, localPort int, mapping portMapping) (<-chan string, error) {
	ssmDocumentName := "DeliverooSSMPortForward"
	params := make(map[string][]string)
	params["portNumber"] = []string{strconv.Itoa(int(mapping.host))}
	params["localPortNumber"] = []string{strconv.Itoa(localPort)}
	jsonParams, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	buffer := bytes.Buffer{}
	_, err = buffer.Write(jsonParams)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "aws", "ssm", "start-session", "--target", mapping.ec2InstanceID, "--document-name", ssmDocumentName, "--parameters", buffer.String())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	stdoutScanner := bufio.NewScanner(stdout)
	stderrScanner := bufio.NewScanner(stderr)
	msgStream := make(chan string)
	go func() {
		defer close(msgStream)
		for stdoutScanner.Scan() || stderrScanner.Scan() {
			t1 := stdoutScanner.Text()
			t2 := stderrScanner.Text()

			if t1 != "" {
				msgStream <- t1
			}
			if t2 != "" {
				msgStream <- t2
				break
			}
		}
		fmt.Printf("Cmd quit")
	}()
	return msgStream, nil
}
