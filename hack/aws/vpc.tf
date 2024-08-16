resource "aws_vpc" "eks" {
	cidr_block = "10.23.0.0/16"
	enable_dns_support = true
	enable_dns_hostnames = true

	tags = {
		Name = "eks"
	}
}

data "aws_availability_zones" "available" {}

resource "aws_subnet" "eks" {
	count = 2

	vpc_id = aws_vpc.eks.id
	cidr_block = cidrsubnet(aws_vpc.eks.cidr_block, 4, count.index)
	availability_zone = data.aws_availability_zones.available.names[count.index]
	map_public_ip_on_launch = true

	tags = {
		Name = "eks - ${count.index}"
	}
}

resource "aws_internet_gateway" "eks" {
	vpc_id = aws_vpc.eks.id

	tags = {
		Name = "eks"
	}
}

resource "aws_route_table" "eks" {
	vpc_id = aws_vpc.eks.id

	route {
		cidr_block = "0.0.0.0/0"
		gateway_id = aws_internet_gateway.eks.id
	}

	tags = {
		Name = "eks"
	}
}

resource "aws_route_table_association" "eks" {
	count = 2
	subnet_id = aws_subnet.eks[count.index].id
	route_table_id = aws_route_table.eks.id
}
