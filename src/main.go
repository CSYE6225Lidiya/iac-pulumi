package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"github.com/dspinhirne/netaddr-go"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/dynamodb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/rds"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/route53"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/sns"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/serviceaccount"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/storage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"
)

type Config struct {
	EncryptionSalt string `yaml:"encryptionsalt"`
	Network        struct {
		CIDRBlockAddr                 string `yaml:"cidrBlockAddr"`
		VPCName                       string `yaml:"vpcName"`
		InternetGateWayName           string `yaml:"internetGatewayName"`
		InternetGatewayAttachmentName string `yaml:"internetGatewayAttachmentName"`
		PublicRouteTableName          string `yaml:"publicRouteTableName"`
		PrivateRouteTableName         string `yaml:"privateRouteTableName"`
		SubNet                        uint   `yaml:"subnet"`
		PublicRouteName               string `yaml:"publicRouteName"`
		SSHKeyName                    string `yaml:"sshKeyName"`
		AmiName                       string `yaml:"amiName"`
		GcpBucketname                 string `yaml:"gcpbucketName"`
		MandrillKey                   string `yaml:"mandrillKey"`
	} `yaml:"network"`
}

func main() {

	// Read the Pulumi yaml file
	yamlFile, err := ioutil.ReadFile("Pulumi.demo.yaml")
	if err != nil {
		log.Fatalf("Error reading YAML file: %v", err)
	}
	var config Config
	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		log.Fatalf("Error parsing YAML: %v", err)
	}

	fmt.Println("EncryptionSalt:", config.EncryptionSalt)
	fmt.Println("CIDR Block Address:", config.Network.CIDRBlockAddr)
	// Parse the VPC CIDR
	net, _ := netaddr.ParseIPv4Net(config.Network.CIDRBlockAddr)

	pulumi.Run(func(ctx *pulumi.Context) error {

		// Get number of availability zones
		available, err := aws.GetAvailabilityZones(ctx, &aws.GetAvailabilityZonesArgs{
			State: pulumi.StringRef("available"),
		}, nil)
		if err != nil {
			return err
		}
		println("AllAvailability", available.AllAvailabilityZones)
		println("Count", len(available.Names))
		noOfAvailabilityZones := len(available.Names)

		// Create VPC
		myVpc, err := ec2.NewVpc(ctx, config.Network.VPCName, &ec2.VpcArgs{
			CidrBlock: pulumi.String(config.Network.CIDRBlockAddr),
		})
		if err != nil {
			return err
		}

		// Create Internet Gateway
		internetGateway, err := ec2.NewInternetGateway(ctx, config.Network.InternetGateWayName, nil)
		if err != nil {
			return err
		}

		// Create Internet Gateway Attachment
		_, err = ec2.NewInternetGatewayAttachment(ctx, config.Network.InternetGatewayAttachmentName, &ec2.InternetGatewayAttachmentArgs{
			InternetGatewayId: internetGateway.ID(),
			VpcId:             myVpc.ID(),
		})
		if err != nil {
			return err
		}

		// Create Public Route Table
		publicRouteTable, err := ec2.NewRouteTable(ctx, config.Network.PublicRouteTableName, &ec2.RouteTableArgs{
			VpcId: myVpc.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("Public Route Table"),
			},
		})
		if err != nil {
			return err
		}

		// Create Private Route Table
		privateRouteTable, err := ec2.NewRouteTable(ctx, config.Network.PrivateRouteTableName, &ec2.RouteTableArgs{
			VpcId: myVpc.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("Private Route Table"),
			},
		})
		if err != nil {
			return err
		}
		var publicSubnetIDs []pulumi.IDOutput
		var privateSubnetIDs []pulumi.IDOutput

		// Create Subnets
		subnetRange := 1
		if noOfAvailabilityZones >= 3 {
			for i := 1; i <= 3; i++ {
				subnetName := "publicSubnet-" + available.Names[i-1]

				subnet, subNetErr := ec2.NewSubnet(ctx, subnetName, &ec2.SubnetArgs{
					VpcId:            myVpc.ID(),
					CidrBlock:        pulumi.String(net.NthSubnet(config.Network.SubNet, uint32(subnetRange)).String()),
					AvailabilityZone: pulumi.String(available.Names[i-1]),
					Tags: pulumi.StringMap{
						"Name": pulumi.String(subnetName),
					},
				})
				if subNetErr != nil {
					fmt.Println(subNetErr)
					return subNetErr
				}
				subnetRange++
				_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("publicSubnet%d-RouteTableAssociation", i), &ec2.RouteTableAssociationArgs{
					SubnetId:     subnet.ID(),
					RouteTableId: publicRouteTable.ID(),
				})
				publicSubnetIDs = append(publicSubnetIDs, subnet.ID())
				if err != nil {
					return err
				}

				subnetName = "privateSubnet-" + available.Names[i-1]

				subnet, subNetErr = ec2.NewSubnet(ctx, subnetName, &ec2.SubnetArgs{
					VpcId:            myVpc.ID(),
					CidrBlock:        pulumi.String(net.NthSubnet(config.Network.SubNet, uint32(subnetRange)).String()),
					AvailabilityZone: pulumi.String(available.Names[i-1]),
					Tags: pulumi.StringMap{
						"Name": pulumi.String(subnetName),
					},
				})
				if subNetErr != nil {
					fmt.Println(subNetErr)
					return subNetErr
				}
				subnetRange++
				privateSubnetIDs = append(privateSubnetIDs, subnet.ID())
				_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("privateSubnet%d-RouteTableAssociation", i), &ec2.RouteTableAssociationArgs{
					SubnetId:     subnet.ID(),
					RouteTableId: privateRouteTable.ID(),
				})
				if err != nil {
					return err
				}

			}

		} else {

			for i := 1; i <= noOfAvailabilityZones; i++ {
				subnetName := "publicSubnet-" + available.Names[i-1]

				subnet, subNetErr := ec2.NewSubnet(ctx, subnetName, &ec2.SubnetArgs{
					VpcId:            myVpc.ID(),
					CidrBlock:        pulumi.String(net.NthSubnet(config.Network.SubNet, uint32(subnetRange)).String()),
					AvailabilityZone: pulumi.String(available.Names[i-1]),
					Tags: pulumi.StringMap{
						"Name": pulumi.String(subnetName),
					},
				})
				if subNetErr != nil {
					fmt.Println(subNetErr)
					return subNetErr
				}
				subnetRange++
				_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("publicSubnet%d-RouteTableAssociation", i), &ec2.RouteTableAssociationArgs{
					SubnetId:     subnet.ID(),
					RouteTableId: publicRouteTable.ID(),
				})
				publicSubnetIDs = append(publicSubnetIDs, subnet.ID())
				if err != nil {
					return err
				}

				subnetName = "privateSubnet-" + available.Names[i-1]

				subnet, subNetErr = ec2.NewSubnet(ctx, subnetName, &ec2.SubnetArgs{
					VpcId:            myVpc.ID(),
					CidrBlock:        pulumi.String(net.NthSubnet(config.Network.SubNet, uint32(subnetRange)).String()),
					AvailabilityZone: pulumi.String(available.Names[i-1]),
					Tags: pulumi.StringMap{
						"Name": pulumi.String(subnetName),
					},
				})
				if subNetErr != nil {
					fmt.Println(subNetErr)
					return subNetErr
				}
				subnetRange++
				privateSubnetIDs = append(privateSubnetIDs, subnet.ID())
				_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("privateSubnet%d-RouteTableAssociation", i), &ec2.RouteTableAssociationArgs{
					SubnetId:     subnet.ID(),
					RouteTableId: privateRouteTable.ID(),
				})
				if err != nil {
					return err
				}

			}

		}

		// Public Route Creation
		_, err = ec2.NewRoute(ctx, config.Network.PublicRouteName, &ec2.RouteArgs{
			RouteTableId:         publicRouteTable.ID(),
			DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
			GatewayId:            internetGateway.ID(),
		})
		if err != nil {
			return err
		}

		// Create load balancer security group
		lbSecurityGroup, err := ec2.NewSecurityGroup(ctx, "lbSecurityGroup", &ec2.SecurityGroupArgs{
			VpcId: myVpc.ID(),
			Ingress: ec2.SecurityGroupIngressArray{
				ec2.SecurityGroupIngressArgs{
					FromPort: pulumi.Int(80),
					ToPort:   pulumi.Int(80),
					Protocol: pulumi.String("tcp"),
					CidrBlocks: pulumi.StringArray{
						pulumi.String("0.0.0.0/0"),
					},
				},
				ec2.SecurityGroupIngressArgs{
					FromPort: pulumi.Int(443),
					ToPort:   pulumi.Int(443),
					Protocol: pulumi.String("tcp"),
					CidrBlocks: pulumi.StringArray{
						pulumi.String("0.0.0.0/0"),
					},
				},
			},
		})
		if err != nil {
			return err
		}
		_, err = ec2.NewSecurityGroupRule(ctx, "outboundruleLoadBalancer", &ec2.SecurityGroupRuleArgs{
			Type:     pulumi.String("egress"), // Set the type to "egress" for an outbound rule.
			FromPort: pulumi.Int(0),           // You can set these values according to your requirements.
			ToPort:   pulumi.Int(65535),       // For example, this allows all outbound traffic.

			Protocol: pulumi.String("tcp"), // -1 indicates all protocols.
			//	SourceSecurityGroupId: appSecGroup.ID(),        // For outbound rules, you typically leave this field empty.
			CidrBlocks: pulumi.StringArray{
				pulumi.String("0.0.0.0/0"),
			},
			SecurityGroupId: lbSecurityGroup.ID(), // The ID of the security group to apply the rule to.
		})
		if err != nil {
			return err
		}

		// Create an application security group for app deployment
		appSecGroup, err := ec2.NewSecurityGroup(ctx, "application security group", &ec2.SecurityGroupArgs{
			Description: pulumi.String("Allow TLS inbound traffic"),
			VpcId:       myVpc.ID(),
		})
		if err != nil {
			return err
		}

		// Create ingress rules for security groups
		// Define the list of ports to allow inbound traffic
		ingressPorts := []int{8080, 22}

		// Add an ingress rule for each port in the list
		for i, port := range ingressPorts {
			_, err := ec2.NewSecurityGroupRule(ctx, fmt.Sprintf("ingressRule-%d", i), &ec2.SecurityGroupRuleArgs{
				Type:                  pulumi.String("ingress"),
				FromPort:              pulumi.Int(port),
				ToPort:                pulumi.Int(port),
				Protocol:              pulumi.String("tcp"),
				SourceSecurityGroupId: lbSecurityGroup.ID(),
				SecurityGroupId:       appSecGroup.ID(),
			})
			if err != nil {
				return err
			}
		}

		// _, err = ec2.NewSecurityGroupRule(ctx, "ingressRuleECSSH", &ec2.SecurityGroupRuleArgs{
		// 	Type:            pulumi.String("ingress"),
		// 	FromPort:        pulumi.Int(22),
		// 	ToPort:          pulumi.Int(22),
		// 	Protocol:        pulumi.String("tcp"),
		// 	CidrBlocks:      pulumi.StringArray{pulumi.String("0.0.0.0/0")},
		// 	SecurityGroupId: appSecGroup.ID(),
		// })
		// if err != nil {
		// 	return err
		// }

		_, err = ec2.NewSecurityGroupRule(ctx, "outboundruleApp", &ec2.SecurityGroupRuleArgs{
			Type:     pulumi.String("egress"),
			FromPort: pulumi.Int(0),
			ToPort:   pulumi.Int(65535),
			Protocol: pulumi.String("tcp"),
			CidrBlocks: pulumi.StringArray{
				pulumi.String("0.0.0.0/0"),
			},
			SecurityGroupId: appSecGroup.ID(),
		})
		if err != nil {
			return err
		}

		// Lookup AMI
		myami, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
			MostRecent: pulumi.BoolRef(true),
			Filters: []ec2.GetAmiFilter{
				{
					Name: "name",
					Values: []string{
						config.Network.AmiName,
					},
				},
			},
		}, nil)
		if err != nil {
			return err
		} else {
			println("&&&&&&&&&&&&&&&&&&&&FoundAMISuccessfully")
		}

		// Create DB Security Group for RDS
		dbSecurityGroup, err := ec2.NewSecurityGroup(ctx, "dbSecurityGroup", &ec2.SecurityGroupArgs{
			Description: pulumi.String("DB Security Group"),
			VpcId:       myVpc.ID(),
		})
		if err != nil {
			return err
		}

		// Add ingress rule to allow traffic on port 3306 (MySQL) or 5432 (PostgreSQL)
		_, err = ec2.NewSecurityGroupRule(ctx, "dbSecurityGroupRule", &ec2.SecurityGroupRuleArgs{
			Type:                  pulumi.String("ingress"),
			FromPort:              pulumi.Int(3306),
			ToPort:                pulumi.Int(3306),
			Protocol:              pulumi.String("tcp"),
			SourceSecurityGroupId: appSecGroup.ID(),
			SecurityGroupId:       dbSecurityGroup.ID(),
		})
		if err != nil {
			return err
		}

		_, err = ec2.NewSecurityGroupRule(ctx, "dbSecurityGroupOutboundRule", &ec2.SecurityGroupRuleArgs{
			Type:                  pulumi.String("egress"),
			FromPort:              pulumi.Int(0),
			ToPort:                pulumi.Int(65535),
			Protocol:              pulumi.String("tcp"),
			SourceSecurityGroupId: appSecGroup.ID(),
			SecurityGroupId:       dbSecurityGroup.ID(),
		})
		if err != nil {
			return err
		}

		// Create Parameter group for RDS
		dbParamGp, err := rds.NewParameterGroup(ctx, "rdsparamgroup", &rds.ParameterGroupArgs{
			Family: pulumi.String("mysql8.0"),
		})
		if err != nil {
			return err
		}

		dbgroupIds := pulumi.StringArray{
			dbSecurityGroup.ID(),
		}

		// Convert pulumi.IDOutput to pulumi.StringArray
		var privateSubnetIDStrings pulumi.StringArray
		for _, id := range privateSubnetIDs {
			privateSubnetIDStrings = append(privateSubnetIDStrings, id.ToStringOutput().ApplyT(func(s string) string { return s }).(pulumi.StringOutput))
		}
		dbPvtSubnetGroup, err := rds.NewSubnetGroup(ctx, "dbsubnetgroup", &rds.SubnetGroupArgs{
			SubnetIds: privateSubnetIDStrings, // Use the private subnets
			Tags: pulumi.StringMap{
				"Name": pulumi.String("MyDBSubnetGroup"),
			},
		})
		if err != nil {
			return err
		}
		myRdsInstance, err := rds.NewInstance(ctx, "rdsinstance", &rds.InstanceArgs{
			AllocatedStorage:    pulumi.Int(10),
			DbName:              pulumi.String("csye6225"),
			Engine:              pulumi.String("mysql"),
			EngineVersion:       pulumi.String("8.0"),
			InstanceClass:       pulumi.String("db.t3.micro"), //check cheapest
			ParameterGroupName:  dbParamGp.Name,
			Password:            pulumi.String("password"),
			SkipFinalSnapshot:   pulumi.Bool(true),
			Username:            pulumi.String("csye6225"),
			MultiAz:             pulumi.Bool(false),
			Identifier:          pulumi.String("csye6225"),
			DbSubnetGroupName:   dbPvtSubnetGroup.Name,
			PubliclyAccessible:  pulumi.Bool(false),
			VpcSecurityGroupIds: dbgroupIds,
		})
		if err != nil {
			return err
		}

		rdsEndpoint := myRdsInstance.Endpoint.ApplyT(func(endpoint pulumi.String) string {
			return string(endpoint)
		})

		mysns, err := sns.NewTopic(ctx, "mySNSTopic", nil)
		if err != nil {
			return err
		}

		MySNSStr := mysns.Arn.ApplyT(func(endpoint pulumi.String) string {
			fmt.Println("Printing the ARNStr", endpoint)
			return string(endpoint)
		})

		sa, err := serviceaccount.NewAccount(ctx, "serviceAccount", &serviceaccount.AccountArgs{
			AccountId:   pulumi.String("service-account-id"),
			DisplayName: pulumi.String("Service Account"),
		})
		if err != nil {
			return err
		}
		// GCP SA KEY
		key, err := serviceaccount.NewKey(ctx, "mykey", &serviceaccount.KeyArgs{
			ServiceAccountId: sa.Name,
			PublicKeyType:    pulumi.String("TYPE_X509_PEM_FILE"),
		})
		if err != nil {
			return err
		}

		// GCP BUCKET IAM
		_, err = storage.NewBucketIAMMember(ctx, "member", &storage.BucketIAMMemberArgs{
			Bucket: pulumi.String(config.Network.GcpBucketname),
			Role:   pulumi.String("roles/storage.objectAdmin"),
			Member: pulumi.String("serviceAccount:service-account-id@demoproject-406516.iam.gserviceaccount.com"),
		}, pulumi.DependsOn([]pulumi.Resource{sa}))
		if err != nil {
			return err
		}
		privateKeyStr := key.PrivateKey.ApplyT(func(endpoint pulumi.String) string {
			fmt.Println("Printing the keystr", endpoint)
			return string(endpoint)
		})
		mydynamodb, err := dynamodb.NewTable(ctx, "dynamotbl", &dynamodb.TableArgs{
			Attributes: dynamodb.TableAttributeArray{
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("ID"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("Name"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("Email"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("DownloadStatus"),
					Type: pulumi.String("S"),
				},
				&dynamodb.TableAttributeArgs{
					Name: pulumi.String("UploadPath"),
					Type: pulumi.String("S"),
				},
			},
			HashKey:       pulumi.String("ID"),
			ReadCapacity:  pulumi.Int(5),
			WriteCapacity: pulumi.Int(5),
			GlobalSecondaryIndexes: dynamodb.TableGlobalSecondaryIndexArray{
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("Name"),
					ProjectionType: pulumi.String("ALL"),
					ReadCapacity:   pulumi.Int(5),
					WriteCapacity:  pulumi.Int(5),
					HashKey:        pulumi.String("Name"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("Email"),
					ProjectionType: pulumi.String("ALL"),
					ReadCapacity:   pulumi.Int(5),
					WriteCapacity:  pulumi.Int(5),
					HashKey:        pulumi.String("Email"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("DownloadStatus"),
					ProjectionType: pulumi.String("ALL"),
					ReadCapacity:   pulumi.Int(5),
					WriteCapacity:  pulumi.Int(5),
					HashKey:        pulumi.String("DownloadStatus"),
				},
				&dynamodb.TableGlobalSecondaryIndexArgs{
					Name:           pulumi.String("UploadPath"),
					ProjectionType: pulumi.String("ALL"),
					ReadCapacity:   pulumi.Int(5),
					WriteCapacity:  pulumi.Int(5),
					HashKey:        pulumi.String("UploadPath"),
				},
			},
		})
		if err != nil {
			return err
		}
		dynamoNameStr := mydynamodb.Name.ApplyT(func(endpoint pulumi.String) string {
			fmt.Println("Printing the keystr", endpoint)
			return string(endpoint)
		})

		pulumi.All(privateKeyStr, MySNSStr, dynamoNameStr).ApplyT(func(all []interface{}) error {
			myPvtKey := all[0].(string)
			mySNS := all[1].(string)
			myDynamoName := all[2].(string)

			fmt.Println("_____________________")
			fmt.Println(myPvtKey)
			fmt.Println("_____________________")

			// Lambda here

			//	roleArn := "arn:aws:iam::203689115380:role/awslambdafullaccess"
			// Create IAM Role
			roleLambda, err := iam.NewRole(ctx, "role-Lambda", &iam.RoleArgs{
				AssumeRolePolicy: pulumi.String(`{
					"Version": "2012-10-17",
					"Statement": [
						{
							"Action": "sts:AssumeRole",
							"Principal": {
								"Service": "lambda.amazonaws.com"
							},
							"Effect": "Allow",
							"Sid": ""
						}
					]
				}`),
			})
			if err != nil {
				return err
			}

			//Attach Lambda Access Policy
			_, err = iam.NewRolePolicyAttachment(ctx, "rolePolicyAttachment-LambdaFullAccess-L", &iam.RolePolicyAttachmentArgs{
				Role:      roleLambda.Name,
				PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AWSLambda_FullAccess"),
			})
			if err != nil {
				return err
			}

			//Attach Lambda Access Policy
			_, err = iam.NewRolePolicyAttachment(ctx, "rolePolicyAttachment-LambdaExecutionPolicy-L", &iam.RolePolicyAttachmentArgs{
				Role:      roleLambda.Name,
				PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
			})
			if err != nil {
				return err
			}

			//Attach DynamoDb Access Policy
			_, err = iam.NewRolePolicyAttachment(ctx, "rolePolicyAttachment-DynamoDBAccess-L", &iam.RolePolicyAttachmentArgs{
				Role:      roleLambda.Name,
				PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonDynamoDBFullAccess"),
			})
			if err != nil {
				return err
			}

			// create Lambda function
			lf, err := lambda.NewFunction(ctx, "myLambdaFunction", &lambda.FunctionArgs{
				Code:    pulumi.NewFileArchive("./myFunction.zip"),
				Handler: pulumi.String("main"), // suitable as per your function's start file
				Role:    roleLambda.Arn,
				Runtime: pulumi.String("go1.x"),
				Timeout: pulumi.Int(60), // Modifiable as per your function's requirement
				Environment: &lambda.FunctionEnvironmentArgs{
					Variables: pulumi.StringMap{
						"GCPKEY":      pulumi.String(myPvtKey),
						"GCBUCKET":    pulumi.String(config.Network.GcpBucketname),
						"DYNAMOTB":    pulumi.String(myDynamoName),
						"MANDRILLKEY": pulumi.String(config.Network.MandrillKey),
					},
				},
			})
			if err != nil {
				return err
			}

			_, err = lambda.NewPermission(ctx, "myLambdaPermission", &lambda.PermissionArgs{
				Action:    pulumi.String("lambda:InvokeFunction"),
				Function:  lf.Name,
				Principal: pulumi.String("sns.amazonaws.com"),
				//SourceArn:   pulumi.String("arn:aws:sns:us-east-1:203689115380:topiceast"),
				SourceArn:   pulumi.String(mySNS),
				StatementId: pulumi.String("MyStatementId"),
			})
			if err != nil {
				return err
			}

			_, err = sns.NewTopicSubscription(ctx, "mySubscription", &sns.TopicSubscriptionArgs{
				Endpoint: lf.Arn,
				Protocol: pulumi.String("lambda"),
				//Topic:    pulumi.StringPtr("arn:aws:sns:us-east-1:203689115380:topiceast"),
				Topic: pulumi.String(mySNS),
			})

			if err != nil {
				return err
			}

			return nil
		})

		// Create EC2 from AMI
		groupIds := pulumi.StringArray{
			appSecGroup.ID(),
		}

		pulumi.All(rdsEndpoint, MySNSStr).ApplyT(func(all []interface{}) error {
			myendPt := all[0].(string)
			mySNS := all[1].(string)
			parts := strings.Split(myendPt, ":")
			var hostname string
			var port string
			if len(parts) == 2 {
				hostname = parts[0]
				port = parts[1]

				fmt.Println("Hostname:", hostname)
				fmt.Println("Port:", port)
			}

			userData := fmt.Sprintf(`#!/bin/bash
				ENV_FILE="/opt/dbconfig.yaml"
				echo user: csye6225 >> ${ENV_FILE}
				echo password: password >> ${ENV_FILE}
				echo host: "%s" >> ${ENV_FILE}
				echo port: 3306 >> ${ENV_FILE}
				echo db: csye6225 >> ${ENV_FILE}
				echo snsarn: "%s" >> ${ENV_FILE}
				sudo chown csye6225:csye6225 $ENV_FILE
				chmod 664 $ENV_FILE
				sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config  -m ec2 -c file:/opt/cloudwatch-config.json -s
			`, hostname, mySNS)

			// Create IAM Role
			role, err := iam.NewRole(ctx, "role", &iam.RoleArgs{
				AssumeRolePolicy: pulumi.String(`{
						"Version": "2012-10-17",
						"Statement": [
							{
								"Action": "sts:AssumeRole",
								"Principal": {
									"Service": "ec2.amazonaws.com"
								},
								"Effect": "Allow",
								"Sid": ""
							}
						]
					}`),
			})
			if err != nil {
				return err
			}
			// Attach 'CloudWatchAgentServerPolicy' to the IAM Role
			_, err = iam.NewRolePolicyAttachment(ctx, "rolePolicyAttachment", &iam.RolePolicyAttachmentArgs{
				Role:      role.Name,
				PolicyArn: pulumi.String("arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"),
			})
			if err != nil {
				return err
			}

			//Attach Lambda Access Policy
			_, err = iam.NewRolePolicyAttachment(ctx, "rolePolicyAttachment-LambdaFullAccess", &iam.RolePolicyAttachmentArgs{
				Role:      role.Name,
				PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AWSLambda_FullAccess"),
			})
			if err != nil {
				return err
			}

			//Attach Lambda Access Policy
			_, err = iam.NewRolePolicyAttachment(ctx, "rolePolicyAttachment-LambdaExecutionPolicy", &iam.RolePolicyAttachmentArgs{
				Role:      role.Name,
				PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
			})
			if err != nil {
				return err
			}

			// Creating the IAM Policy for SNS Publish
			snsPublishPolicy, err := iam.NewPolicy(ctx, "snsPublishPolicy", &iam.PolicyArgs{
				Policy: pulumi.String(`{
					"Version": "2012-10-17",
					"Statement": [{
						"Effect": "Allow",
						"Action": "sns:Publish",
						"Resource": "*"
					}]
				}`),
			})
			if err != nil {
				return err
			}

			// Start creating IAM Role policy attachmentpulumi destroy

			_, err = iam.NewRolePolicyAttachment(ctx, "rolePolicyAttachment-snspublish", &iam.RolePolicyAttachmentArgs{
				Role:      role.Name,
				PolicyArn: snsPublishPolicy.Arn,
			})
			if err != nil {
				return err
			}

			// Create IAM Instance Profile and connect role
			instanceProfile, err := iam.NewInstanceProfile(ctx, "instanceProfile", &iam.InstanceProfileArgs{
				Role: role.Name,
			})
			if err != nil {
				return err
			}

			userData1 := base64.StdEncoding.EncodeToString([]byte(userData))

			// Create Launch Template
			launchTemplate, err := ec2.NewLaunchTemplate(ctx, "launchTemplate", &ec2.LaunchTemplateArgs{
				ImageId:      pulumi.String(myami.Id),
				UserData:     pulumi.String(userData1),
				InstanceType: pulumi.String("t2.micro"),
				NetworkInterfaces: ec2.LaunchTemplateNetworkInterfaceArray{
					ec2.LaunchTemplateNetworkInterfaceArgs{
						AssociatePublicIpAddress: pulumi.String("true"),
						SecurityGroups:           groupIds,
					},
				},
				KeyName: pulumi.String(config.Network.SSHKeyName),
				IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileArgs{
					Name: instanceProfile.Name,
				},
				Name: pulumi.String("launchTemplate"),
				//VpcSecurityGroupIds: groupIds,

			})
			if err != nil {
				return err
			}

			subnetIds := pulumi.StringArray{publicSubnetIDs[0], publicSubnetIDs[1], publicSubnetIDs[2]}

			// Create AutoScaling Group
			asg, err := autoscaling.NewGroup(ctx, "asg", &autoscaling.GroupArgs{
				LaunchTemplate: &autoscaling.GroupLaunchTemplateArgs{
					Id: launchTemplate.ID(),
				},
				MinSize:                pulumi.Int(1),
				MaxSize:                pulumi.Int(3),
				DesiredCapacity:        pulumi.Int(1),
				DefaultCooldown:        pulumi.Int(60),
				VpcZoneIdentifiers:     subnetIds,
				HealthCheckGracePeriod: pulumi.Int(400),
				Tags: autoscaling.GroupTagArray{
					&autoscaling.GroupTagArgs{
						Key:               pulumi.String("AutoScaleTag"),
						Value:             pulumi.String("AutoScaleGpTag"),
						PropagateAtLaunch: pulumi.Bool(true),
					},
				},
				Name: pulumi.String("asg"),
			})
			if err != nil {
				return err
			}

			// Create AutoScaling Policy - ScaleUp
			policyUp, err := autoscaling.NewPolicy(ctx, "scaleUp", &autoscaling.PolicyArgs{
				AdjustmentType:       pulumi.String("ChangeInCapacity"),
				ScalingAdjustment:    pulumi.Int(1),
				PolicyType:           pulumi.String("SimpleScaling"),
				AutoscalingGroupName: asg.Name,
			})
			if err != nil {
				return err
			}

			_, err = cloudwatch.NewMetricAlarm(ctx, "cpuHigh", &cloudwatch.MetricAlarmArgs{
				ComparisonOperator: pulumi.String("GreaterThanThreshold"),
				EvaluationPeriods:  pulumi.Int(2),
				MetricName:         pulumi.String("CPUUtilization"),
				Namespace:          pulumi.String("AWS/EC2"),
				Period:             pulumi.Int(60),
				Statistic:          pulumi.String("Average"),
				Threshold:          pulumi.Float64(5.0),
				Dimensions: pulumi.StringMap{
					"AutoScalingGroupName": asg.Name,
				},
				AlarmDescription: pulumi.String("This metric monitors ec2 cpu utilization and scales up"),
				AlarmActions: pulumi.Array{
					policyUp.Arn,
				},
			})
			if err != nil {
				return err
			}

			// Create AutoScaling Policy - ScaleDn
			policyDn, err := autoscaling.NewPolicy(ctx, "scaleDn", &autoscaling.PolicyArgs{
				AdjustmentType:       pulumi.String("ChangeInCapacity"),
				ScalingAdjustment:    pulumi.Int(-1),
				PolicyType:           pulumi.String("SimpleScaling"),
				AutoscalingGroupName: asg.Name,
			})
			if err != nil {
				return err
			}

			_, err = cloudwatch.NewMetricAlarm(ctx, "cpuLow", &cloudwatch.MetricAlarmArgs{
				ComparisonOperator: pulumi.String("LessThanThreshold"),
				EvaluationPeriods:  pulumi.Int(2),
				MetricName:         pulumi.String("CPUUtilization"),
				Namespace:          pulumi.String("AWS/EC2"),
				Period:             pulumi.Int(60),
				Statistic:          pulumi.String("Average"),
				Threshold:          pulumi.Float64(3.0),
				Dimensions: pulumi.StringMap{
					"AutoScalingGroupName": asg.Name,
				},
				AlarmDescription: pulumi.String("This metric monitors ec2 cpu utilization and scales up"),
				AlarmActions: pulumi.Array{
					policyDn.Arn,
				},
			})
			if err != nil {
				return err
			}

			// Create load balancer
			apl, err := lb.NewLoadBalancer(ctx, "testloadBalancer", &lb.LoadBalancerArgs{
				Internal:         pulumi.Bool(false),
				LoadBalancerType: pulumi.String("application"),
				SecurityGroups: pulumi.StringArray{
					lbSecurityGroup.ID(),
				},
				Subnets:                  subnetIds,
				EnableDeletionProtection: pulumi.Bool(false),

				Tags: pulumi.StringMap{
					"Environment": pulumi.String("production"),
				},
			})
			if err != nil {
				return err
			}

			// Create target group
			targetGroup, err := lb.NewTargetGroup(ctx, "testTargetgroup", &lb.TargetGroupArgs{
				Port:     pulumi.Int(8080), // app listening port
				Protocol: pulumi.String("HTTP"),
				VpcId:    myVpc.ID(),
				HealthCheck: &lb.TargetGroupHealthCheckArgs{
					Enabled:  pulumi.Bool(true),
					Interval: pulumi.Int(30),
					Path:     pulumi.String("/healthz"),
					Timeout:  pulumi.Int(5),
					Port:     pulumi.String("traffic-port"), // default is "traffic-port"
					Protocol: pulumi.String("HTTP"),         // default is the same as the 'Protocol' field above
					Matcher:  pulumi.String("200"),          // default is "200", for HTTP and HTTPS.
				},
			})
			if err != nil {
				return err
			}

			_, err = autoscaling.NewAttachment(ctx, "targpattachment", &autoscaling.AttachmentArgs{
				AutoscalingGroupName: asg.Name,
				LbTargetGroupArn:     targetGroup.Arn,
			})
			if err != nil {
				return err
			}

			// // Attach a listener to the load balancer
			// _, err = lb.NewListener(ctx, "myListenerALB", &lb.ListenerArgs{
			// 	DefaultActions: lb.ListenerDefaultActionArray{
			// 		&lb.ListenerDefaultActionArgs{
			// 			TargetGroupArn: targetGroup.Arn,
			// 			Type:           pulumi.String("forward"),
			// 		},
			// 	},
			// 	LoadBalancerArn: apl.Arn,
			// 	Port:            pulumi.Int(80),
			// 	Protocol:        pulumi.String("HTTP"),
			// })
			// if err != nil {
			// 	return err
			// }

			_, err = lb.NewListener(ctx, "myListenerALB", &lb.ListenerArgs{
				DefaultActions: lb.ListenerDefaultActionArray{
					&lb.ListenerDefaultActionArgs{
						TargetGroupArn: targetGroup.Arn,
						Type:           pulumi.String("forward"),
					},
				},
				LoadBalancerArn: apl.Arn,
				Port:            pulumi.Int(443),
				SslPolicy:       pulumi.String("ELBSecurityPolicy-2016-08"),
				CertificateArn:  pulumi.String("arn:aws:acm:us-east-1:785896633607:certificate/c21cc8df-5f58-42ee-bf77-c60e053c27ae"),
				Protocol:        pulumi.String("HTTPS"),
			})
			if err != nil {
				return err
			}

			zoneID := "Z0420517820XQJZJL7G9"

			// Create a A record with the public IP of the EC2 instance
			_, err = route53.NewRecord(ctx, "record", &route53.RecordArgs{
				Name: pulumi.String("demo.lidiyacloud.me"), // replace with your domain
				Type: pulumi.String("A"),
				Aliases: route53.RecordAliasArray{
					&route53.RecordAliasArgs{
						Name:                 apl.DnsName,
						ZoneId:               apl.ZoneId,
						EvaluateTargetHealth: pulumi.Bool(false),
					},
				},
				ZoneId: pulumi.String(zoneID),
			})

			if err != nil {
				return err
			}

			// println("***********Successfully created ec2 from ami")
			return nil
		})

		return nil
	})
}
