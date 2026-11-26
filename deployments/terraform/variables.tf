variable "aws_region" {
  description = "Região da AWS"
  type        = string
  default     = "us-east-2"
}

variable "aws_access_key" {
  description = "AWS Access Key"
  type        = string
  sensitive   = true
}

variable "aws_secret_key" {
  description = "AWS Secret Key"
  type        = string
  sensitive   = true
}

variable "ssh_public_key_path" {
    type        = string
    default = "~/.ssh/opspulse-aws"
}

variable "ssh_private_key_path" {
  type = string
}

variable "ssh_key_name" {
    type = string
}

variable "my_public_ip" {
  type = string
}