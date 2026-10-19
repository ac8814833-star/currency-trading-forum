param(
    [switch]$RetrievalOnly,
    [ValidateRange(0, 35)][int]$Limit = 0
)

$previousLocation = Get-Location
$trackedEnvNames = @('RAG_DASHSCOPE_KEY', 'RAG_EVAL_DB', 'RAG_EVAL_COLLECTION', 'RAG_EVAL_OUTPUT', 'RAG_EVAL_RETRIEVAL_ONLY', 'RAG_EVAL_LIMIT')
$previousEnv = @{}
foreach ($name in $trackedEnvNames) {
    $previousEnv[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$secureKey = Read-Host 'DashScope API Key' -AsSecureString
$keyPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureKey)
try {
    Set-Location (Split-Path $PSScriptRoot -Parent)
    $env:RAG_DASHSCOPE_KEY = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($keyPointer)
    $env:RAG_EVAL_DB = 'rag_eval_20260926134627_sw6f'
    $env:RAG_EVAL_COLLECTION = 'rag_eval_qwen37_20260926134627_sw6f'
    $env:RAG_EVAL_OUTPUT = if ($RetrievalOnly) { 'rag_eval_reports/retrieval_qwen37_rerun.json' } else { 'rag_eval_reports/full_qwen37_rerun.json' }
    $env:RAG_EVAL_RETRIEVAL_ONLY = if ($RetrievalOnly) { '1' } else { '0' }
    $env:RAG_EVAL_LIMIT = if ($Limit -gt 0) { [string]$Limit } else { '' }
    go run ./.rag-eval-run/main.go
}
finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($keyPointer)
    foreach ($name in $trackedEnvNames) {
        [Environment]::SetEnvironmentVariable($name, $previousEnv[$name], 'Process')
    }
    Set-Location $previousLocation
}
