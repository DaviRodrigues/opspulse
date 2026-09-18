output "ec2_public_ip" {
  description = "IP Público da instância EC2 para acesso SSH"
  value       = aws_instance.opspulse_ec2.public_ip
}

output "ssh_command" {
  description = "Comando pronto para conectar via SSH"
  value = "ssh -i ${var.ssh_public_key_path} ubuntu@${aws_instance.opspulse_ec2.public_ip}"
}