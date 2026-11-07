import { Node } from "./api"

export function writable(node: Node) {
  if (node.type === "variable") return true
  if (node.type !== "modbusIo") return false
  const ioType = node.points.find((point) => point.type === "modbusIoType")?.text
  const readOnly = node.points.find((point) => point.type === "readOnly")?.value || 0
  return readOnly === 0 && (ioType === "modbusCoil" || ioType === "modbusHoldingRegister")
}
