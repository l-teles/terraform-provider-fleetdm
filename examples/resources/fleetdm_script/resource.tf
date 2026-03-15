# Manage a shell script for macOS/Linux hosts
resource "fleetdm_script" "security_check" {
  name    = "security-compliance-check.sh"
  content = <<-EOF
    #!/bin/bash
    # Security compliance check script
    
    # Check if FileVault is enabled (macOS)
    if [[ "$OSTYPE" == "darwin"* ]]; then
      fdesetup status
    fi
    
    # Check if firewall is enabled
    if command -v ufw &> /dev/null; then
      sudo ufw status
    fi
    
    echo "Security check complete"
  EOF
}

# Script assigned to a specific team
resource "fleetdm_script" "team_script" {
  name    = "team-maintenance.sh"
  team_id = fleetdm_team.workstations.id
  content = <<-EOF
    #!/bin/bash
    echo "Running team-specific maintenance..."
    # Add maintenance commands here
  EOF
}

# PowerShell script for Windows hosts
resource "fleetdm_script" "windows_script" {
  name    = "windows-audit.ps1"
  content = <<-EOF
    # Windows security audit script
    Write-Host "Running Windows security audit..."
    
    # Check Windows Defender status
    Get-MpComputerStatus | Select-Object -Property AntivirusEnabled, RealTimeProtectionEnabled
    
    # Check BitLocker status
    Get-BitLockerVolume | Select-Object -Property MountPoint, ProtectionStatus
  EOF
}
