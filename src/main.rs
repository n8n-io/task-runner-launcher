use std::{ffi::CString, ptr::null};

use clap::{Parser, Subcommand};
use libc::{c_char, chdir, setgid, setuid};
#[cfg(feature = "secure-mode")]
use libc::{getegid, geteuid};
use log::debug;
use serde::Deserialize;

#[derive(Deserialize, Debug, Clone)]
#[serde(rename_all = "kebab-case")]
struct TaskRunnerConfig {
    runner_type: String,
    workdir: String,
    command: String,
    args: Vec<String>,
    allowed_env: Vec<String>,
    uid: u32,
    gid: u32,
}

#[derive(Deserialize, Debug, Clone)]
#[serde(rename_all = "kebab-case")]
struct LauncherConfig {
    // ws_url: String,
    task_runners: Vec<TaskRunnerConfig>,
}
const EXPECTED_UID: u32 = 0;
const EXPECTED_GID: u32 = 0;

fn set_uid_and_gid(uid: u32, gid: u32) {
    unsafe {
        // We need to call this in reverse if we're already root
        // otherwise the setgid call fails
        if geteuid() == 0 {
            setgid(gid);
            setuid(uid);
        } else {
            setuid(uid);
            setgid(gid);
        }

        #[cfg(feature = "secure-mode")]
        {
            let effective_uid = geteuid();
            let effective_gid = getegid();
            if effective_uid != uid {
                panic!("Expected user ID {}, instead got {}", uid, effective_uid);
            } else if effective_gid != gid {
                panic!("Expected group ID {}, instead got {}", gid, effective_gid);
            }
        }
    }
}

#[derive(Parser, Debug)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand, Debug)]
enum Commands {
    Launch {
        #[arg(index = 1)]
        runner_type: String,
    },
    Kill {
        #[arg(index = 1)]
        runner_type: String,

        #[arg(index = 2)]
        pid: libc::pid_t,
    },
}

unsafe fn get_errno_str() -> String {
    let reason = CString::from_raw(libc::strerror(
        std::io::Error::last_os_error().raw_os_error().unwrap(),
    ));
    reason.to_str().unwrap().to_string()
}

fn launch_runner(config: TaskRunnerConfig) {
    let default_envs: Vec<String> = vec!["LANG".into(), "PATH".into(), "TZ".into(), "TERM".into()];
    unsafe {
        debug!("Setting uid ({}) and gid ({})", config.uid, config.gid);
        set_uid_and_gid(config.uid, config.gid);

        let workdir = CString::new(config.workdir.clone()).unwrap();
        if chdir(workdir.as_ptr()) != 0 {
            panic!(
                "Failed to chdir into configured directory ({}) with error \"{}\". Exiting.",
                config.workdir,
                get_errno_str()
            );
        }
        debug!("chdir to {}", config.workdir);

        let command = CString::new(config.command).unwrap();

        let envs = std::env::vars()
            .filter(|(name, _)| default_envs.contains(name) || config.allowed_env.contains(name))
            .map(|(name, val)| CString::new(format!("{}={}", name, val)).unwrap())
            .collect::<Vec<CString>>();
        let mut c_envs = envs
            .iter()
            .map(|env| env.as_ptr())
            .collect::<Vec<*const c_char>>();
        c_envs.push(null());

        debug!("envs built: {:?}", envs);

        let args = config
            .args
            .iter()
            .map(|arg| CString::new(arg.as_str()).unwrap())
            .collect::<Vec<CString>>();
        let mut c_args = args
            .iter()
            .map(|arg| arg.as_ptr())
            .collect::<Vec<*const c_char>>();
        c_args.insert(0, command.as_ptr());
        c_args.push(null());
        debug!("args built: {:?}", args);

        debug!("Executing runner (execve)");
        libc::execve(command.as_ptr(), c_args.as_ptr(), c_envs.as_ptr());
    }
}

fn kill_runner(config: TaskRunnerConfig, pid: libc::pid_t) {
    unsafe {
        debug!("Setting uid ({}) and gid ({})", config.uid, config.gid);
        set_uid_and_gid(config.uid, config.gid);

        if libc::kill(pid, libc::SIGTERM) != 0 {
            panic!(
                "Failed to kill task runner with reason \"{}\"",
                get_errno_str()
            );
        }

        debug!("Runner killed successfully");
    }
}

#[cfg(not(feature = "secure-mode"))]
pub const LAUNCHER_CONFIG_PATH: &str = "./config.json";

#[cfg(feature = "secure-mode")]
pub const LAUNCHER_CONFIG_PATH: &str = "/etc/n8n-task-runners.json";

fn main() {
    env_logger::init();
    let cli = Cli::parse();

    let runner_type = match &cli.command {
        Commands::Launch { runner_type } => runner_type,
        Commands::Kill { runner_type, .. } => runner_type,
    };

    debug!("Got runner type: {}", runner_type);

    let config_str: String = std::fs::read_to_string(LAUNCHER_CONFIG_PATH).unwrap_or_else(|_| {
        panic!("Failed to open config: {}", LAUNCHER_CONFIG_PATH,);
    });
    let config: LauncherConfig =
        serde_json::from_str(&config_str).expect("Failed to parse launcher config file");

    debug!("Parsed launcher config");

    let runner_config = config
        .task_runners
        .iter()
        .find(|c| &c.runner_type == runner_type)
        .expect(format!("Unknown runner type: {}", runner_type).as_str())
        .clone();
    debug!("Found runner config");

    debug!("Attempting to escalate to root");
    // Try escalate to root, then fail if in secure mode
    set_uid_and_gid(EXPECTED_UID, EXPECTED_GID);

    match cli.command {
        Commands::Launch { .. } => {
            debug!("Launching runner");
            launch_runner(runner_config);
        }
        Commands::Kill { pid, .. } => {
            debug!("Killing runner");
            kill_runner(runner_config, pid);
        }
    }
}
