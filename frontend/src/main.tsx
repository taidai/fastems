import { createRoot } from "react-dom/client"
import { setupIonicReact } from "@ionic/react"
import "@ionic/react/css/core.css"
import App from "./App"
import "./tokens.css"
import "./tailwind.css"
import "./styles.css"
import "./reference.css"
import "./components/ems/ems.css"

setupIonicReact()
createRoot(document.getElementById("root")!).render(<App />)
