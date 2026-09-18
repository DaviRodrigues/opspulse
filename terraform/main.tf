resource "aws_key_pair" "opspulse_key" {
  key_name   = var.ssh_key_name
  public_key = file(pathexpand(var.ssh_public_key_path))
}

resource "aws_default_vpc" "default" {}

resource "aws_security_group" "opspulse_sg" {
  name        = "opspulse_security_group"
  description = "Allow inbound SSH trafic, and all outbound traffic"
  vpc_id      = aws_default_vpc.default.id

  ingress {
    description = "Allow SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.my_public_ip]
  }

  egress {
    description      = "Allow all outbound traffic"
    from_port        = 0
    to_port          = 0
    protocol         = "-1" # -1 same 'all protocols'
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = {
    Name = "OpspulseSG"
  }
}

data "aws_ami" "ubuntu_free_tier" {
  most_recent = true
  owners      = ["099720109477"]

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_instance" "opspulse_ec2" {
  ami           = data.aws_ami.ubuntu_free_tier.id
  instance_type = "t3.micro"
  key_name = aws_key_pair.opspulse_key.key_name
  vpc_security_group_ids = [ aws_security_group.opspulse_sg.id ]
  associate_public_ip_address = true

  tags = {
    Name = "OpspulseEc2"
  }
}
