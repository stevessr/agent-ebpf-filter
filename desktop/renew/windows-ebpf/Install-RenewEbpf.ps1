# Optional, explicit administrator-run eBPF for Windows hook setup.
[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [Parameter(Mandatory = $true)][string]$IncludeDir,
    [ValidateSet("Verify", "Install")][string]$Action = "Verify",
    [string]$Clang = "clang",
    [string]$Netsh = "netsh"
)
$ErrorActionPreference = "Stop"
$source = Join-Path $PSScriptRoot "renew_connect.c"
$obj = Join-Path $PSScriptRoot "renew_connect.o"
if (-not (Test-Path -LiteralPath $IncludeDir)) {
    throw "eBPF for Windows include directory not found: $IncludeDir"
}
if ($PSCmdlet.ShouldProcess($obj, "Compile eBPF for Windows hooks")) {
    & $Clang -target bpf -O2 -g -Werror -I $IncludeDir -c $source -o $obj
    if ($LASTEXITCODE -ne 0) { throw "Clang eBPF compilation failed" }
}
if ($PSCmdlet.ShouldProcess($obj, "Verify eBPF sections")) {
    & $Netsh ebpf show sections $obj
    if ($LASTEXITCODE -ne 0) { throw "eBPF for Windows provider missing or program incompatible" }
}
if ($Action -eq "Install" -and $PSCmdlet.ShouldProcess($obj, "Attach eBPF programs")) {
    # Some eBPF for Windows releases only load signed native .sys programs;
    # this command must fail visibly instead of masking a hook failure.
    & $Netsh ebpf add program $obj
    if ($LASTEXITCODE -ne 0) {
        throw "Runtime rejected ELF. Follow installed eBPF for Windows release instructions for native .sys conversion/signing."
    }
    & $Netsh ebpf show programs
    if ($LASTEXITCODE -ne 0) { throw "Cannot verify eBPF program status" }
}
