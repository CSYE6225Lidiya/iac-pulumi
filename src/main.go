package main

import (
	"fmt"
	"io/ioutil"
	"log"

	"github.com/dspinhirne/netaddr-go"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
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

		// Create EC2 from AMI

		groupIds := pulumi.StringArray{
			appSecGroup.ID(),
		}

		_, err = ec2.NewInstance(ctx, "web", &ec2.InstanceArgs{
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
		})
		if err != nil {
			return err
		} else {
			println("***********Successfully created ec2 from ami")
		}

		return nil
	})
}
