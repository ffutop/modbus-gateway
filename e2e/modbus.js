// Minimal Modbus TCP client for driving traffic through the gateway in tests.
const net = require('node:net');

let txn = 0;

function request(port, unit, pdu) {
  return new Promise((resolve, reject) => {
    const sock = net.connect(port, '127.0.0.1');
    const id = ++txn & 0xffff;
    const mbap = Buffer.alloc(7);
    mbap.writeUInt16BE(id, 0);
    mbap.writeUInt16BE(0, 2);
    mbap.writeUInt16BE(pdu.length + 1, 4);
    mbap.writeUInt8(unit, 6);
    let buf = Buffer.alloc(0);
    sock.setTimeout(3000, () => sock.destroy(new Error('modbus timeout')));
    sock.on('error', reject);
    sock.on('data', (d) => {
      buf = Buffer.concat([buf, d]);
      if (buf.length >= 6 && buf.length >= 6 + buf.readUInt16BE(4)) {
        sock.end();
        const resp = buf.subarray(7, 6 + buf.readUInt16BE(4));
        if (resp[0] & 0x80) reject(new Error(`modbus exception ${resp[1]}`));
        else resolve(resp);
      }
    });
    sock.write(Buffer.concat([mbap, pdu]));
  });
}

exports.readHolding = async (port, unit, address, count) => {
  const pdu = Buffer.from([3, address >> 8, address & 0xff, count >> 8, count & 0xff]);
  const resp = await request(port, unit, pdu);
  return Array.from({ length: count }, (_, i) => resp.readUInt16BE(2 + i * 2));
};

exports.writeRegister = (port, unit, address, value) =>
  request(port, unit, Buffer.from([6, address >> 8, address & 0xff, value >> 8, value & 0xff]));
