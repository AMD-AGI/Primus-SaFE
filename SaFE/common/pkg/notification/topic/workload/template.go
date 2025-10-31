/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

const emailTemplate = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8" />
    <title>Workload Notification</title>
  </head>
  <body style="font-family: Arial, sans-serif; background-color: #f7f9fc; margin: 0; padding: 0;">
    <table width="100%" cellpadding="0" cellspacing="0" style="max-width: 600px; margin: 30px auto; background-color: #ffffff; border-radius: 10px; box-shadow: 0 2px 6px rgba(0,0,0,0.05);">
      <tr>
        <td style="background-color: #2b6cb0; color: #ffffff; padding: 16px 24px; border-top-left-radius: 10px; border-top-right-radius: 10px;">
          <h2 style="margin: 0;">📢 Workload Notification</h2>
        </td>
      </tr>
      <tr>
        <td style="padding: 24px;">
          <p>Hello,</p>
          <p>Your workload status has been updated:</p>

          <table cellpadding="6" cellspacing="0" width="100%" style="border-collapse: collapse; margin-top: 10px;">
            <tr>
              <td width="120" style="color: #555; font-weight: bold;">Workload Id</td>
              <td>{{.JobName}}</td>
            </tr>
            <tr>
              <td style="color: #555; font-weight: bold;">Status</td>
              <td style="color: {{.StatusColor}}; font-weight: bold;">{{.Status}}</td>
            </tr>
            <tr>
              <td style="color: #555; font-weight: bold;">Scheduled At</td>
              <td>{{.ScheduleTime}}</td>
            </tr>
            {{if .ErrorMessage}}
            <tr>
              <td style="color: #555; font-weight: bold;">Error</td>
              <td style="color: #c53030;">{{.ErrorMessage}}</td>
            </tr>
            {{end}}
          </table>

          <p style="margin-top: 20px;">
             You can view more details on the console:<br />
            <a href="{{.JobURL}}" style="color: #2b6cb0; text-decoration: none;">{{.JobURL}}</a>
          </p>

          <p style="margin-top: 30px; color: #666;">
            --<br />
            Primus SaFE<br />
            <br />(Stability and Fault Endurance)<br />
            (This is an automated message, please do not reply.)
          </p>
        </td>
      </tr>
    </table>
  </body>
</html>`
