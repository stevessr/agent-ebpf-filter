fn main() {
    let proto_dir = "../../../proto";
    println!("cargo:rerun-if-changed={proto_dir}");
    prost_build::Config::new()
        .compile_protos(&[format!("{proto_dir}/tracker.proto")], &[proto_dir])
        .expect("compile tracker protobuf schema");
}
