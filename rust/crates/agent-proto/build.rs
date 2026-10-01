fn main() {
    let proto_dir = "../../../proto";
    println!("cargo:rerun-if-changed={proto_dir}");

    let protos = [
        format!("{proto_dir}/tracker_common.proto"),
        format!("{proto_dir}/tracker_events.proto"),
        format!("{proto_dir}/tracker_registration.proto"),
        format!("{proto_dir}/tracker_system.proto"),
        format!("{proto_dir}/tracker_config.proto"),
        format!("{proto_dir}/tracker_shell.proto"),
    ];

    prost_build::Config::new()
        .compile_protos(&protos, &[proto_dir])
        .expect("compile tracker protobuf schema");
}
