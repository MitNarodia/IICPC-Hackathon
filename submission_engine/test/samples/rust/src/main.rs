use std::io::Write;
use std::net::TcpListener;

fn main() {
    let listener = TcpListener::bind("0.0.0.0:8080").unwrap();
    for stream in listener.incoming() {
        let mut stream = stream.unwrap();
        let _ = stream.write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok");
    }
}
