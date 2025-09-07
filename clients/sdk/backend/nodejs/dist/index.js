"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.BackendClient = void 0;
exports.default = ProgressDB;
const client_1 = __importDefault(require("./client"));
exports.BackendClient = client_1.default;
// ProgressDB factory returns a ready-to-use client instance.
function ProgressDB(opts) {
    return new client_1.default(opts);
}
