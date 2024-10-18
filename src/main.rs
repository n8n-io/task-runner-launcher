use std::{
    ffi::CString,
    ptr::{null, null_mut},
};

use clap::{Parser, Subcommand};
use libc::{c_char, chdir, geteuid, setgid, setgroups, setuid};
#[cfg(feature = "secure-mode")]
use libc::{getegid, getgroups};
use log::debug;
use serde::Deserialize;
use serde_json::json;
use ureq;

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
            setgroups(0, null());
            setuid(uid);
        } else {
            setuid(uid);
            setgid(gid);
            setgroups(0, null());
        }

        #[cfg(feature = "secure-mode")]
        {
            let effective_uid = geteuid();
            let effective_gid = getegid();
            let sup_groups = getgroups(0, null_mut());
            if effective_uid != uid {
                panic!("Expected user ID {}, instead got {}", uid, effective_uid);
            } else if effective_gid != gid {
                panic!("Expected group ID {}, instead got {}", gid, effective_gid);
            } else if sup_groups != 0 {
                panic!(
                    "Expected 0 supplementary groups, instead got {}",
                    sup_groups
                );
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
    let reason_orig = CString::from_raw(libc::strerror(
        std::io::Error::last_os_error().raw_os_error().unwrap(),
    ));
    let reason = reason_orig.clone();
    // We need to forget this, otherwise Rust will try to free it which
    // result in a segfault
    std::mem::forget(reason_orig);
    let reason_str = reason.to_str().unwrap();
    reason_str.to_string()
}

fn launch_runner(config: TaskRunnerConfig, grant_token: Option<String>) {
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

        let mut envs = std::env::vars()
            .filter(|(name, _)| default_envs.contains(name) || config.allowed_env.contains(name))
            .map(|(name, val)| CString::new(format!("{}={}", name, val)).unwrap())
            .collect::<Vec<CString>>();

        if let Some(token) = grant_token {
            envs.retain(|env| {
                !env.to_str()
                    .unwrap()
                    .starts_with("N8N_RUNNERS_GRANT_TOKEN=")
            });
            envs.push(CString::new(format!("N8N_RUNNERS_GRANT_TOKEN={}", token)).unwrap());
        }

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

        // In theory, this should never happen, but just in case something goes wrong,
        // we should give a decent error
        if libc::execve(command.as_ptr(), c_args.as_ptr(), c_envs.as_ptr()) == -1 {
            let err_str = get_errno_str();
            panic!(
                "Failed to execute task runner with error \"{}\". Exiting.",
                err_str
            );
        }
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

fn fetch_grant_token(
    n8n_uri: &str,
    auth_token: &str,
) -> Result<String, Box<dyn std::error::Error>> {
    let url = format!("http://{}/runners/auth", n8n_uri);
    debug!("Fetching grant token from url {}", url);

    let result = ureq::post(&url).send_json(json!({ "token": auth_token }));

    return match result {
        Ok(response) => {
            let body: serde_json::Value = response.into_json()?;
            Ok(body["data"]["token"].as_str().unwrap().to_string())
        }
        Err(e) => Err(Box::new(ureq::Error::from(e))),
    };
}

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

    let grant_token = if let Ok(auth_token) = std::env::var("N8N_RUNNERS_AUTH_TOKEN") {
        let n8n_uri = match std::env::var("N8N_RUNNERS_N8N_URI") {
            Ok(uri) => uri,
            Err(_) => {
                panic!("Error: N8N_RUNNERS_N8N_URI is required when N8N_RUNNERS_AUTH_TOKEN is set");
            }
        };

        match fetch_grant_token(&n8n_uri, &auth_token) {
            Ok(token) => Some(token),
            Err(e) => {
                panic!("Error: Failed to fetch grant token: {}", e);
            }
        }
    } else {
        None
    };

    match cli.command {
        Commands::Launch { .. } => {
            debug!("Launching runner");
            launch_runner(runner_config, grant_token);
        }
        Commands::Kill { pid, .. } => {
            debug!("Killing runner");
            kill_runner(runner_config, pid);
        }
    }
}
