package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"github.com/dspinhirne/netaddr-go"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/rds"
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
		ingressPorts := []int{22, 80, 443, 8080}

		// Add an ingress rule for each port in the list
		for i, port := range ingressPorts {
			_, err := ec2.NewSecurityGroupRule(ctx, fmt.Sprintf("ingressRule-%d", i), &ec2.SecurityGroupRuleArgs{
				Type:            pulumi.String("ingress"),
				FromPort:        pulumi.Int(port),
				ToPort:          pulumi.Int(port),
				Protocol:        pulumi.String("tcp"),
				CidrBlocks:      pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				SecurityGroupId: appSecGroup.ID(),
			})
			if err != nil {
				return err
			}
		}

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
		// Create EC2 from AMI
		groupIds := pulumi.StringArray{
			appSecGroup.ID(),
		}

		pulumi.All(rdsEndpoint).ApplyT(func(all []interface{}) error {
			myendPt := all[0].(string)
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
				sudo chown csye6225:csye6225 $ENV_FILE
				chmod 664 $ENV_FILE
			`, hostname)

			_, err := ec2.NewInstance(ctx, "web", &ec2.InstanceArgs{
				Ami:                   pulumi.String(myami.Id),
				InstanceType:          pulumi.String("t2.micro"),
				DisableApiTermination: pulumi.Bool(false),
				RootBlockDevice: &ec2.InstanceRootBlockDeviceArgs{
					DeleteOnTermination: pulumi.Bool(true),
					VolumeSize:          pulumi.Int(25),
					VolumeType:          pulumi.String("gp2"),
				},
				VpcSecurityGroupIds:      groupIds,
				SubnetId:                 publicSubnetIDs[0],
				AssociatePublicIpAddress: pulumi.Bool(true),
				KeyName:                  pulumi.String(config.Network.SSHKeyName),
				UserData:                 pulumi.String(userData),
			}, pulumi.DependsOn([]pulumi.Resource{myRdsInstance}))

			if err != nil {
				return err
			}

			println("***********Successfully created ec2 from ami")
			return nil
		})

		return nil
	})
}
