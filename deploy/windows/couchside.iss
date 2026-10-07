; Couchside for Windows: installs couchside.exe with ffmpeg beside it, writes
; %ProgramData%\Couchside\couchside.env, and runs it as the "Couchside"
; Windows service. Built by .github/workflows/windows-installer.yml:
;
;   ISCC /DAppVersion=v0.13.0 /DNumVersion=0.13.0 /DStage=<folder> /DOutputDir=<folder> couchside.iss
;
; <Stage> holds couchside.exe, ffmpeg.exe, ffprobe.exe, LICENSE,
; THIRD_PARTY_NOTICES.txt and FFMPEG.txt. Data (database, cache, logs) lives
; in %ProgramData%\Couchside and is kept on uninstall.

#ifndef AppVersion
  #define AppVersion "dev"
#endif
#ifndef NumVersion
  #define NumVersion "0.0.0"
#endif
#ifndef Stage
  #define Stage "stage"
#endif
#ifndef OutputDir
  #define OutputDir "out"
#endif

#define ServiceName "Couchside"

[Setup]
AppId={{8C1F7B0E-5D3A-4E61-9B7A-2F4C6D8E1A35}
AppName=Couchside
AppVersion={#AppVersion}
AppVerName=Couchside {#AppVersion}
AppPublisher=Couchside
AppPublisherURL=https://couchside.app
AppSupportURL=https://github.com/timothydodd/couchside/issues
VersionInfoVersion={#NumVersion}
DefaultDirName={autopf}\Couchside
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir={#OutputDir}
OutputBaseFilename=couchside-{#AppVersion}-windows-amd64-setup
SetupIconFile=couchside.ico
UninstallDisplayIcon={app}\couchside.ico
UninstallDisplayName=Couchside
WizardStyle=modern
Compression=lzma2/max
SolidCompression=yes
; The service is stopped in PrepareToInstall, so files aren't in use.
CloseApplications=no

[Tasks]
Name: firewall; Description: "Let TVs, phones and other computers on your network connect (Windows Firewall)"
Name: desktopicon; Description: "Create a desktop shortcut"; Flags: unchecked

[Dirs]
Name: "{commonappdata}\Couchside"
Name: "{commonappdata}\Couchside\data"

[Files]
Source: "{#Stage}\couchside.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Stage}\ffmpeg.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Stage}\ffprobe.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "couchside.ico"; DestDir: "{app}"
Source: "{#Stage}\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"
Source: "{#Stage}\THIRD_PARTY_NOTICES.txt"; DestDir: "{app}"
Source: "{#Stage}\FFMPEG.txt"; DestDir: "{app}"

[INI]
Filename: "{app}\Couchside.url"; Section: "InternetShortcut"; Key: "URL"; String: "http://localhost:{code:GetPort}/"

[Icons]
Name: "{autoprograms}\Couchside"; Filename: "{app}\Couchside.url"; IconFilename: "{app}\couchside.ico"
Name: "{autodesktop}\Couchside"; Filename: "{app}\Couchside.url"; IconFilename: "{app}\couchside.ico"; Tasks: desktopicon

[Run]
; One rule for the program covers the web port and LAN discovery (UDP 1900).
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Couchside"""; Flags: runhidden
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Couchside"" dir=in action=allow program=""{app}\couchside.exe"" enable=yes profile=private,domain"; Flags: runhidden; Tasks: firewall
Filename: "http://localhost:{code:GetPort}/"; Description: "Open Couchside in your browser"; Flags: postinstall shellexec nowait skipifsilent

[UninstallRun]
; net stop waits for the service to stop, so its files can be removed.
Filename: "{sys}\net.exe"; Parameters: "stop {#ServiceName}"; Flags: runhidden; RunOnceId: "StopService"
Filename: "{sys}\sc.exe"; Parameters: "delete {#ServiceName}"; Flags: runhidden; RunOnceId: "DeleteService"
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Couchside"""; Flags: runhidden; RunOnceId: "DeleteFirewallRule"

[UninstallDelete]
Type: files; Name: "{app}\Couchside.url"

[Code]
const
  DRIVE_REMOTE = 4;
  ERROR_SERVICE_EXISTS = 1073;
  ERROR_SERVICE_ALREADY_RUNNING = 1056;

var
  MediaPage: TInputDirWizardPage;
  PortPage: TInputQueryWizardPage;
  Gpus: String;

function GetDriveType(lpRootPathName: String): Cardinal;
  external 'GetDriveTypeW@kernel32.dll stdcall';

function EnvFile: String;
begin
  Result := ExpandConstant('{commonappdata}\Couchside\couchside.env');
end;

function DataDir: String;
begin
  Result := ExpandConstant('{commonappdata}\Couchside\data');
end;

function Unquote(S: String): String;
begin
  S := Trim(S);
  if (Length(S) >= 2) and (S[1] = '"') and (S[Length(S)] = '"') then
    S := Copy(S, 2, Length(S) - 2);
  Result := S;
end;

{ ReadSetting is KEY's value in the existing settings file, so an upgrade
  offers what's set now. }
function ReadSetting(Key, Default: String): String;
var
  Lines: TArrayOfString;
  I: Integer;
begin
  Result := Default;
  if not LoadStringsFromFile(EnvFile, Lines) then
    Exit;
  for I := 0 to GetArrayLength(Lines) - 1 do
    if Pos(Uppercase(Key) + '=', Uppercase(Trim(Lines[I]))) = 1 then
      Result := Unquote(Copy(Trim(Lines[I]), Length(Key) + 2, MaxInt));
end;

{ SetSetting replaces KEY's line, or adds one. }
procedure SetSetting(var Lines: TArrayOfString; Key, Value: String);
var
  I, N: Integer;
begin
  N := GetArrayLength(Lines);
  for I := 0 to N - 1 do
    if Pos(Uppercase(Key) + '=', Uppercase(Trim(Lines[I]))) = 1 then
    begin
      Lines[I] := Key + '=' + Value;
      Exit;
    end;
  SetArrayLength(Lines, N + 1);
  Lines[N] := Key + '=' + Value;
end;

function HasSetting(Lines: TArrayOfString; Key: String): Boolean;
var
  I: Integer;
begin
  Result := False;
  for I := 0 to GetArrayLength(Lines) - 1 do
    if Pos(Uppercase(Key) + '=', Uppercase(Trim(Lines[I]))) = 1 then
      Result := True;
end;

function GetPort(Param: String): String;
begin
  Result := Trim(PortPage.Values[0]);
end;

{ PortFromAddr turns COUCHSIDE_ADDR (":8080", "0.0.0.0:8080") into its port. }
function PortFromAddr(Addr: String): String;
var
  I: Integer;
begin
  Result := Addr;
  for I := Length(Addr) downto 1 do
    if Addr[I] = ':' then
    begin
      Result := Copy(Addr, I + 1, MaxInt);
      Exit;
    end;
end;

{ NvidiaDriver turns Windows' driver version (32.0.15.9649) into NVIDIA's
  own numbering (596.49): the last five digits. }
function NvidiaDriver(V: String): String;
var
  Digits: String;
  I: Integer;
begin
  Digits := '';
  for I := 1 to Length(V) do
    if (V[I] >= '0') and (V[I] <= '9') then
      Digits := Digits + V[I];
  if Length(Digits) < 5 then
    Result := V
  else
  begin
    Digits := Copy(Digits, Length(Digits) - 4, 5);
    Result := IntToStr(StrToIntDef(Copy(Digits, 1, 3), 0)) + '.' + Copy(Digits, 4, 2);
  end;
end;

{ DetectGpus lists the graphics adapters and what Couchside does with each.
  The server tests the encoders itself at start-up; this only tells the user
  what to expect. }
function DetectGpus: String;
var
  Locator, Services, Items, Item: Variant;
  I: Integer;
  Name, Line: String;
begin
  Result := '';
  try
    Locator := CreateOleObject('WbemScripting.SWbemLocator');
    Services := Locator.ConnectServer('.', 'root\CIMV2');
    Items := Services.ExecQuery('SELECT Name, DriverVersion FROM Win32_VideoController');
    for I := 0 to Items.Count - 1 do
    begin
      Item := Items.ItemIndex(I);
      Name := Item.Name;
      if Pos('NVIDIA', Uppercase(Name)) > 0 then
        Line := Name + ' (driver ' + NvidiaDriver(Item.DriverVersion) + '): transcodes with NVENC'
      else if Pos('INTEL', Uppercase(Name)) > 0 then
        Line := Name + ': transcodes with Quick Sync'
      else if (Pos('AMD', Uppercase(Name)) > 0) or (Pos('RADEON', Uppercase(Name)) > 0) then
        Line := Name + ': not used yet (the CPU transcodes)'
      else
        Continue;
      Result := Result + '      ' + Line + #13#10;
    end;
  except
    Result := '';
  end;
  if Result = '' then
    Result := '      None found: the CPU transcodes' + #13#10;
end;

procedure InitializeWizard;
begin
  MediaPage := CreateInputDirPage(wpSelectDir, 'Media folder', 'Where are your movies and TV shows?',
    'Pick the folder that holds your media folders (for example D:\Media). When you add a library, Couchside lets you browse inside it.' + #13#10#13#10 +
    'For a NAS, use its network path (\\nas\media), not a mapped drive letter: the service can''t see mapped drives. ' +
    'Leave it empty to allow any folder (you''ll type each library''s path).',
    False, '');
  MediaPage.Add('');
  MediaPage.Values[0] := ReadSetting('COUCHSIDE_MEDIA_ROOT', ExpandConstant('{%USERPROFILE}\Videos'));

  PortPage := CreateInputQueryPage(MediaPage.ID, 'Port', 'Which port should Couchside use?',
    'Browsers and TV apps connect on this port. Keep 8080 unless something else already uses it.');
  PortPage.Add('Port:', False);
  PortPage.Values[0] := PortFromAddr(ReadSetting('COUCHSIDE_ADDR', ':8080'));

  Gpus := DetectGpus;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  Media: String;
  Port: Integer;
begin
  Result := True;
  if CurPageID = MediaPage.ID then
  begin
    Media := Trim(MediaPage.Values[0]);
    if Media = '' then
      Exit;
    if (Length(Media) >= 2) and (Media[2] = ':') and (GetDriveType(Copy(Media, 1, 2) + '\') = DRIVE_REMOTE) then
    begin
      MsgBox(Copy(Media, 1, 2) + ' is a mapped network drive, which the Couchside service can''t see. ' +
        'Use the share''s network path instead, for example \\nas\media.', mbError, MB_OK);
      Result := False;
    end
    else if not DirExists(Media) then
      Result := MsgBox(Media + ' doesn''t exist. Use it anyway?', mbConfirmation, MB_YESNO) = IDYES;
  end
  else if CurPageID = PortPage.ID then
  begin
    Port := StrToIntDef(Trim(PortPage.Values[0]), 0);
    if (Port < 1) or (Port > 65535) then
    begin
      MsgBox('Enter a port number from 1 to 65535.', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo, MemoTypeInfo,
  MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
var
  Media: String;
begin
  Media := Trim(MediaPage.Values[0]);
  if Media = '' then
    Media := '(any folder)';
  Result := MemoDirInfo + NewLine + NewLine +
    'Media folder:' + NewLine + Space + Media + NewLine + NewLine +
    'Web address:' + NewLine + Space + 'http://localhost:' + GetPort('') + NewLine + NewLine +
    'Data (database, artwork, logs), kept if you uninstall:' + NewLine + Space + DataDir + NewLine + NewLine +
    'Graphics:' + NewLine + Gpus;
  if MemoTasksInfo <> '' then
    Result := Result + NewLine + MemoTasksInfo;
end;

function ServiceExec(Params: String): Integer;
begin
  if not Exec(ExpandConstant('{sys}\sc.exe'), Params, '', SW_HIDE, ewWaitUntilTerminated, Result) then
    Result := -1;
end;

{ Stop a running service before its files are replaced. }
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Code: Integer;
begin
  Exec(ExpandConstant('{sys}\net.exe'), 'stop {#ServiceName}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Result := '';
end;

{ WriteSettings keeps everything already in couchside.env and sets the media
  folder and port. The data folder is only added when the file has none. }
procedure WriteSettings;
var
  Lines: TArrayOfString;
begin
  if not LoadStringsFromFile(EnvFile, Lines) then
  begin
    SetArrayLength(Lines, 4);
    Lines[0] := '# Couchside settings: one KEY=value per line. All of them are in';
    Lines[1] := '# docs/configuration.md (https://github.com/timothydodd/couchside). After editing,';
    Lines[2] := '# restart the service: Services (services.msc) -> Couchside -> Restart.';
    Lines[3] := '';
  end;
  SetSetting(Lines, 'COUCHSIDE_MEDIA_ROOT', Trim(MediaPage.Values[0]));
  SetSetting(Lines, 'COUCHSIDE_ADDR', ':' + GetPort(''));
  if not HasSetting(Lines, 'COUCHSIDE_DATA_DIR') then
    SetSetting(Lines, 'COUCHSIDE_DATA_DIR', DataDir);
  if not SaveStringsToUTF8FileWithoutBOM(EnvFile, Lines, False) then
    MsgBox('Couldn''t write ' + EnvFile + '.', mbError, MB_OK);
end;

procedure InstallService;
var
  Bin: String;
  Code: Integer;
begin
  Bin := 'binPath= "\"' + ExpandConstant('{app}\couchside.exe') + '\"" start= delayed-auto DisplayName= "Couchside"';
  Code := ServiceExec('create {#ServiceName} ' + Bin);
  if Code = ERROR_SERVICE_EXISTS then
    Code := ServiceExec('config {#ServiceName} ' + Bin);
  if Code <> 0 then
  begin
    MsgBox('Couldn''t register the Couchside service (sc.exe error ' + IntToStr(Code) + ').', mbError, MB_OK);
    Exit;
  end;
  ServiceExec('description {#ServiceName} "Couchside media server"');
  { Restart after a crash or a failed start (failureflag counts a non-zero exit). }
  ServiceExec('failure {#ServiceName} reset= 86400 actions= restart/5000/restart/10000/restart/60000');
  ServiceExec('failureflag {#ServiceName} 1');
  Code := ServiceExec('start {#ServiceName}');
  if (Code <> 0) and (Code <> ERROR_SERVICE_ALREADY_RUNNING) then
    MsgBox('The Couchside service didn''t start (sc.exe error ' + IntToStr(Code) + '). ' +
      'Its log is in ' + DataDir + '\logs.', mbError, MB_OK);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    WriteSettings;
    InstallService;
  end;
end;

procedure CurPageChanged(CurPageID: Integer);
begin
  if CurPageID = wpFinished then
    WizardForm.FinishedLabel.Caption :=
      'Couchside is running as a Windows service and starts with Windows.' + #13#10#13#10 +
      'Open http://localhost:' + GetPort('') + ' here, or http://' + GetComputerNameString + ':' + GetPort('') +
      ' from another device. Your GPU, if Couchside can use it, is shown in Settings -> System.' + #13#10#13#10 +
      'Media on a NAS: the service runs as Local System, which usually can''t read network shares. ' +
      'In Services (services.msc), open Couchside -> Log On, pick an account that can read the share, then restart the service.' + #13#10#13#10 +
      'Settings: ' + EnvFile;
end;
