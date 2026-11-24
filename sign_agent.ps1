$cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject "CN=Studio Foxxo Internal Signer, O=Studio Foxxo, C=FR" -CertStoreLocation "Cert:\CurrentUser\My" -NotAfter (Get-Date).AddYears(5)
Export-Certificate -Cert $cert -FilePath ".\StudioFoxxo_RootCA.cer" -Force
Set-AuthenticodeSignature -FilePath ".\bin\agent.exe" -Certificate $cert -HashAlgorithm SHA256 -TimestampServer "http://timestamp.digicert.com"
Get-AuthenticodeSignature ".\bin\agent.exe" | Format-List
