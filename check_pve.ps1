[System.Net.ServicePointManager]::ServerCertificateValidationCallback = {$true}

$usernames = @('AGY@pve', 'agy@pve', 'AGY@pam', 'agy@pam', 'root@pam')

foreach ($u in $usernames) {
    try {
        $body = @{ username = $u; password = 'agyisthebest' }
        $res = Invoke-RestMethod -Uri "https://192.168.4.16:8006/api2/json/access/ticket" -Method Post -Body $body -TimeoutSec 4
        Write-Host "[SUCCESS] Identifiants valides avec : $u"
        $res.data | ConvertTo-Json
        break
    } catch {
        Write-Host "[-] Echec pour : $u ($($_.Exception.Message))"
    }
}
