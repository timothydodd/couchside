import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import Dialogs from "./components/Dialogs";
import Notices from "./components/Notices";
import "./index.css";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
    <Dialogs />
    <Notices />
  </React.StrictMode>,
);
