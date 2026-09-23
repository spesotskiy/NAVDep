$ErrorActionPreference = 'Stop'

$src = "C:\GIT\NAV2018\App\Objects"
$dst = "C:\GIT\NAVDep\TestObjects"
[void][System.IO.Directory]::CreateDirectory($dst)

$typeToPrefix = @{
  'Codeunit'  = 'c'
  'Table'     = 't'
  'Page'      = 'p'
  'Report'    = 'r'
  'XMLport'   = 'x'
  'Query'     = 'q'
  'MenuSuite' = 'm'
}

$files = [System.IO.Directory]::GetFiles($src)
$seen = New-Object 'System.Collections.Generic.HashSet[string]'
$errors = New-Object System.Collections.Generic.List[string]
$slashCount = 0
$copied = 0

foreach ($path in $files) {
  $fs = [System.IO.File]::Open($path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
  $sr = $null
  try {
    $sr = New-Object System.IO.StreamReader($fs, (New-Object System.Text.UTF8Encoding $false), $true)
    $line = $sr.ReadLine()
  }
  finally {
    if ($sr) { $sr.Dispose() } else { $fs.Dispose() }
  }

  if ($line -notmatch '^OBJECT\s+(\S+)\s+(\d+)\s+(.*)$') {
    $errors.Add("unparsed: $([System.IO.Path]::GetFileName($path)) :: $line")
    continue
  }

  $type = $Matches[1]
  $id = [int]$Matches[2]
  $name = $Matches[3].Trim()
  if ($name.Length -ge 2 -and $name.StartsWith('"') -and $name.EndsWith('"')) {
    $name = $name.Substring(1, $name.Length - 2)
  }
  if (-not $typeToPrefix.ContainsKey($type)) {
    $errors.Add("unknown type ${type}: $([System.IO.Path]::GetFileName($path))")
    continue
  }
  if ($name.Contains('/')) {
    $slashCount++
    $name = $name.Replace('/', '-')
  }

  $destName = "{0}{1} - {2}.txt" -f $typeToPrefix[$type], $id, $name
  if (-not $seen.Add($destName)) {
    $errors.Add("duplicate: $destName from $([System.IO.Path]::GetFileName($path))")
    continue
  }

  $destPath = [System.IO.Path]::Combine($dst, $destName)
  try {
    [System.IO.File]::Copy($path, $destPath, $false)
    $copied++
  }
  catch {
    $errors.Add("copy failed: $destName :: $($_.Exception.Message)")
  }
}

Write-Host "source: $($files.Length)"
Write-Host "copied: $copied"
Write-Host "slash-renamed: $slashCount"
Write-Host "errors: $($errors.Count)"
$errors | Select-Object -First 30
Write-Host "dest count: $([System.IO.Directory]::GetFiles($dst).Length)"
