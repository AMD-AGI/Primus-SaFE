/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspaceaccess

const emailTemplate = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8" />
    <title>Workspace Access Request</title>
  </head>
  <body style="font-family: Arial, sans-serif; background-color: #f7f9fc; margin: 0; padding: 0;">
    <table width="100%" cellpadding="0" cellspacing="0" style="max-width: 600px; margin: 30px auto; background-color: #ffffff; border-radius: 10px;">
      <tr>
        <td style="background-color: #2b6cb0; color: #ffffff; padding: 16px 24px; border-top-left-radius: 10px; border-top-right-radius: 10px;">
          <h2 style="margin: 0;">Workspace Access Request</h2>
        </td>
      </tr>
      <tr>
        <td style="padding: 24px;">
          <p>Hello,</p>
          <p>{{.Summary}}</p>
          <table cellpadding="6" cellspacing="0" width="100%" style="border-collapse: collapse; margin-top: 10px;">
            <tr>
              <td width="120" style="color: #555; font-weight: bold;">Workspace</td>
              <td>{{.Workspace}}</td>
            </tr>
            <tr>
              <td style="color: #555; font-weight: bold;">Requester</td>
              <td>{{.Requester}}{{if .RequesterMail}} ({{.RequesterMail}}){{end}}</td>
            </tr>
            {{if .Reason}}
            <tr>
              <td style="color: #555; font-weight: bold;">Reason</td>
              <td>{{.Reason}}</td>
            </tr>
            {{end}}
            {{if .DecidedBy}}
            <tr>
              <td style="color: #555; font-weight: bold;">Decided By</td>
              <td>{{.DecidedBy}}</td>
            </tr>
            {{end}}
            {{if .Comment}}
            <tr>
              <td style="color: #555; font-weight: bold;">Comment</td>
              <td>{{.Comment}}</td>
            </tr>
            {{end}}
          </table>
          {{if .URL}}
          <p style="margin-top: 20px;">
            <a href="{{.URL}}" style="background-color: #2b6cb0; color: #ffffff; padding: 10px 18px; border-radius: 6px; text-decoration: none;">Open the request</a>
          </p>
          <p style="color: #777; font-size: 12px;">{{.URL}}</p>
          {{end}}
        </td>
      </tr>
    </table>
  </body>
</html>
`
